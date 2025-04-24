package job

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/Takenobou/yoinker/internal/download"
	"github.com/Takenobou/yoinker/internal/util"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

const (
	maxRetries = 3
	retryDelay = 10 * time.Second
)

// JobStatus represents the current status of a job execution
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

// ActiveJob holds information about a currently running job
type ActiveJob struct {
	JobID     int
	URL       string
	StartTime time.Time
	Status    JobStatus
	Error     error
}

// Scheduler manages the scheduling and execution of download jobs
type Scheduler struct {
	db            *sql.DB            // Database connection for job persistence
	logger        *zap.Logger        // Logger for recording scheduler events
	downloadRoot  string             // Root directory for downloads
	maxConcurrent int                // Maximum number of concurrent downloads
	cronScheduler *gocron.Scheduler  // Underlying cron scheduler
	sem           chan struct{}      // Semaphore for limiting concurrent downloads
	ctx           context.Context    // Context for graceful shutdown
	cancel        context.CancelFunc // Function to cancel the context
	activeJobs    map[int]*ActiveJob // Map of currently active jobs by ID
	jobsMutex     sync.RWMutex       // Mutex for safe concurrent access to the jobs map
}

func NewScheduler(db *sql.DB, logger *zap.Logger, downloadRoot string, maxConcurrent int) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		db:            db,
		logger:        logger,
		downloadRoot:  downloadRoot,
		maxConcurrent: maxConcurrent,
		cronScheduler: gocron.NewScheduler(time.Local),
		sem:           make(chan struct{}, maxConcurrent),
		ctx:           ctx,
		cancel:        cancel,
		activeJobs:    make(map[int]*ActiveJob),
	}
}

// GetActiveJobs returns a copy of all currently active jobs
func (s *Scheduler) GetActiveJobs() []ActiveJob {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	jobs := make([]ActiveJob, 0, len(s.activeJobs))
	for _, job := range s.activeJobs {
		jobs = append(jobs, *job)
	}
	return jobs
}

// GetJobStatus returns the status of a specific job
func (s *Scheduler) GetJobStatus(jobID int) (ActiveJob, bool) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	job, exists := s.activeJobs[jobID]
	if !exists {
		return ActiveJob{}, false
	}
	return *job, true
}

func (s *Scheduler) Start() {
	s.logger.Info("Scheduler starting with gocron")
	s.refreshJobs()

	// Add periodic job refresh (every 5 minutes)
	s.cronScheduler.Every(5).Minutes().Do(func() {
		s.logger.Info("Performing periodic job refresh")
		s.refreshJobs()
	})

	s.cronScheduler.StartAsync()
}

func (s *Scheduler) Stop() {
	s.logger.Info("Scheduler stopping")
	// Cancel the context to signal all operations to stop
	s.cancel()
	s.cronScheduler.Stop()

	// Wait for a short time to allow in-progress downloads to respond to cancellation
	time.Sleep(500 * time.Millisecond)

	// Log any jobs that were still active during shutdown
	s.jobsMutex.RLock()
	activeCount := len(s.activeJobs)
	s.jobsMutex.RUnlock()

	if activeCount > 0 {
		s.logger.Warn("Scheduler stopped with active jobs", zap.Int("activeCount", activeCount))
	}
}

// computeNextStartTime computes the next start time for a given job.
// If the job has never run or the candidate time (LastRun + Interval)
// is before or equal to now, it flags the job as overdue.
func computeNextStartTime(jobItem *Job) (nextStart time.Time, overdue bool) {
	now := time.Now()
	if jobItem.LastRun == nil {
		// Job never ran; schedule for now+interval and mark as overdue.
		return now.Add(time.Second * time.Duration(jobItem.Interval)), true
	}
	candidate := jobItem.LastRun.Add(time.Second * time.Duration(jobItem.Interval))
	if candidate.Before(now) || candidate.Equal(now) {
		return now.Add(time.Second * time.Duration(jobItem.Interval)), true
	}
	return candidate, false
}

// scheduleCron schedules jobItem at startAt with its configured interval.
func (s *Scheduler) scheduleCron(jobItem *Job, startAt time.Time) {
	_, err := s.cronScheduler.
		Every(jobItem.Interval).Seconds().
		StartAt(startAt).
		Do(s.runJobClosure(jobItem))
	if err != nil {
		s.logger.Error("Failed to schedule job", zap.Int("jobID", jobItem.ID), zap.Error(err))
	}
}

// runJobClosure returns the closure to execute a scheduled job with semaphore and cancellation.
func (s *Scheduler) runJobClosure(jobItem *Job) func() {
	return func() {
		jobCtx, cancel := context.WithTimeout(s.ctx, time.Duration(jobItem.Interval)*time.Second)
		defer cancel()

		select {
		case s.sem <- struct{}{}:
			s.executeJobWithContext(jobCtx, jobItem)
			<-s.sem
		case <-s.ctx.Done():
			s.logger.Info("Scheduler context canceled, skipping job execution",
				zap.Int("jobID", jobItem.ID))
		}
	}
}

// refreshJobs loads all enabled jobs from the database, and schedules each job
// based on its stored LastRun. If a job is overdue, it triggers an immediate download.
func (s *Scheduler) refreshJobs() {
	s.cronScheduler.Clear()

	jobs, err := ListJobs(s.db)
	if err != nil {
		s.logger.Error("Failed to list jobs", zap.Error(err))
		return
	}

	for _, jobItem := range jobs {
		if !jobItem.Enabled {
			continue
		}

		jobToSchedule := jobItem
		nextStart, overdue := computeNextStartTime(&jobToSchedule)
		s.logger.Info("Scheduling job",
			zap.Int("jobID", jobToSchedule.ID),
			zap.Time("nextStart", nextStart),
			zap.Int("intervalSeconds", jobToSchedule.Interval),
		)

		s.scheduleCron(&jobToSchedule, nextStart)

		// If overdue, trigger an immediate execution in a separate goroutine.
		if overdue {
			s.logger.Info("Job is overdue, executing immediately", zap.Int("jobID", jobToSchedule.ID))
			go s.semWrapperWithContext(s.ctx, &jobToSchedule)
		}
	}
}

// ScheduleJob handles the scheduling of a newly created job.
// If the job is overdue (or has never run), it triggers an immediate download
// and then schedules the recurring execution starting from now + interval.
// Otherwise, it simply schedules the job to start at the computed nextStart.
func (s *Scheduler) ScheduleJob(jobItem *Job) {
	if !jobItem.Enabled {
		return
	}

	nextStart, overdue := computeNextStartTime(jobItem)
	if overdue {
		s.logger.Info("New job is overdue, executing immediately", zap.Int("jobID", jobItem.ID))
		// Execute immediate download in a separate goroutine.
		go s.semWrapperWithContext(s.ctx, jobItem)

		newStart := time.Now().Add(time.Second * time.Duration(jobItem.Interval))
		s.logger.Info("Scheduling recurring job", zap.Int("jobID", jobItem.ID), zap.Time("newStart", newStart))
		s.scheduleCron(jobItem, newStart)
	} else {
		s.logger.Info("Scheduling new job", zap.Int("jobID", jobItem.ID), zap.Time("nextStart", nextStart))
		s.scheduleCron(jobItem, nextStart)
	}
}

// semWrapperWithContext wraps executeJobWithContext with semaphore control
func (s *Scheduler) semWrapperWithContext(ctx context.Context, jobItem *Job) {
	select {
	case s.sem <- struct{}{}:
		s.executeJobWithContext(ctx, jobItem)
		<-s.sem
	case <-ctx.Done():
		s.logger.Info("Context canceled, skipping job execution", zap.Int("jobID", jobItem.ID))
	}
}

// executeJobWithContext attempts to download the job's file (with retries),
// logs the download, and updates the job's LastRun timestamp.
// It respects context cancellation for graceful shutdown.
func (s *Scheduler) executeJobWithContext(ctx context.Context, jobItem *Job) {
	s.logger.Info("Executing job", zap.Int("jobID", jobItem.ID), zap.String("url", jobItem.URL))

	// Check if context is already done before starting work
	if ctx.Err() != nil {
		s.logger.Info("Context canceled before execution", zap.Int("jobID", jobItem.ID))
		return
	}

	dest, err := s.prepareDestination(jobItem)
	if err != nil {
		s.logger.Error("Failed to prepare destination", zap.Int("jobID", jobItem.ID), zap.Error(err))
		return
	}

	var fileHash string
	var execErr error
	startTime := time.Now()

	// Track job status
	s.jobsMutex.Lock()
	s.activeJobs[jobItem.ID] = &ActiveJob{
		JobID:     jobItem.ID,
		URL:       jobItem.URL,
		StartTime: startTime,
		Status:    StatusRunning,
	}
	s.jobsMutex.Unlock()

	// Use a download context to enable cancellation of the current download
	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	downloadComplete := make(chan struct{})
	go func() {
		defer close(downloadComplete)
		fileHash, execErr = s.attemptDownload(downloadCtx, jobItem, dest)
	}()

	// Wait for either download completion or context cancellation
	select {
	case <-downloadComplete:
		// Download completed or failed on its own
	case <-ctx.Done():
		s.logger.Info("Context canceled, aborting download", zap.Int("jobID", jobItem.ID))
		cancel()           // Cancel the download context
		<-downloadComplete // Wait for download goroutine to exit
		execErr = ctx.Err()
	}

	duration := time.Since(startTime)
	if execErr != nil {
		s.logger.Error("Download failed", zap.Int("jobID", jobItem.ID), zap.Error(execErr), zap.Duration("duration", duration))
		s.jobsMutex.Lock()
		s.activeJobs[jobItem.ID].Status = StatusFailed
		s.activeJobs[jobItem.ID].Error = execErr
		s.jobsMutex.Unlock()
		return
	}

	s.logger.Info("Download succeeded", zap.Int("jobID", jobItem.ID), zap.Duration("duration", duration))

	if err := LogDownload(s.db, jobItem.ID, dest, fileHash); err != nil {
		s.logger.Error("Failed to log download", zap.Int("jobID", jobItem.ID), zap.Error(err))
	}

	// Update job's LastRun timestamp.
	jobItem.LastRun = &startTime
	if err := UpdateJob(s.db, *jobItem); err != nil {
		s.logger.Error("Failed to update job last_run", zap.Int("jobID", jobItem.ID), zap.Error(err))
	} else {
		s.logger.Info("Job execution updated", zap.Int("jobID", jobItem.ID))
	}

	// Mark job as completed
	s.jobsMutex.Lock()
	s.activeJobs[jobItem.ID].Status = StatusCompleted
	s.jobsMutex.Unlock()
}

// prepareDestination constructs the download path for a job
func (s *Scheduler) prepareDestination(jobItem *Job) (string, error) {
	// Parse URL path
	parsed, err := url.Parse(jobItem.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	// Create job-specific directory
	jobDir := filepath.Join(s.downloadRoot, fmt.Sprintf("job_%d", jobItem.ID))
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return "", err
	}
	// Determine filename
	filename := path.Base(parsed.Path)
	if filename == "" {
		filename = "downloaded_file"
	}
	filename = util.EnsureSafeFilename(filename)
	return filepath.Join(jobDir, filename), nil
}

// attemptDownload retries downloading up to maxRetries, returning the file hash or error
func (s *Scheduler) attemptDownload(ctx context.Context, jobItem *Job, dest string) (string, error) {
	var fileHash string
	var execErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		s.logger.Info("Attempting download", zap.Int("attempt", attempt), zap.Int("jobID", jobItem.ID))
		fileHash, execErr = download.DownloadFileWithContext(ctx, jobItem.URL, dest, jobItem.Overwrite, s.logger)
		if execErr == nil {
			return fileHash, nil
		}
		s.logger.Warn("Download attempt failed", zap.Int("attempt", attempt), zap.Int("jobID", jobItem.ID), zap.Error(execErr))
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(retryDelay):
		}
	}
	return "", execErr
}
