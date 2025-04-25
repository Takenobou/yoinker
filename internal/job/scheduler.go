package job

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/Takenobou/yoinker/internal/download"
	"github.com/Takenobou/yoinker/internal/util"
	"github.com/go-co-op/gocron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	maxRetries = 3
	retryDelay = 10 * time.Second
)

// Prometheus metrics
var (
	downloadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "yoinker_download_total", Help: "Total download operations",
	}, []string{"status"})
	activeJobsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "yoinker_active_jobs", Help: "Number of active jobs",
	})
	downloadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "yoinker_download_duration_seconds", Help: "Download duration in seconds",
	})
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
	db               *sql.DB            // Database connection for job persistence
	logger           *zap.Logger        // Logger for recording scheduler events
	downloadRoot     string             // Root directory for downloads
	maxConcurrent    int                // Maximum number of concurrent downloads
	allowUnsafeHooks bool               // whether to permit shell-based hook execution
	cronScheduler    *gocron.Scheduler  // Underlying cron scheduler
	sem              chan struct{}      // Semaphore for limiting concurrent downloads
	ctx              context.Context    // Context for graceful shutdown
	cancel           context.CancelFunc // Function to cancel the context
	activeJobs       map[int]*ActiveJob // Map of currently active jobs by ID
	jobsMutex        sync.RWMutex       // Mutex for safe concurrent access to the jobs map
	nowFunc          func() time.Time   // Function to obtain current time (for testing)
}

// NewScheduler creates a Scheduler; allowUnsafeHooks controls hook execution
func NewScheduler(db *sql.DB, logger *zap.Logger, downloadRoot string, maxConcurrent int, allowUnsafeHooks bool) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		db:               db,
		logger:           logger,
		downloadRoot:     downloadRoot,
		maxConcurrent:    maxConcurrent,
		allowUnsafeHooks: allowUnsafeHooks,
		cronScheduler:    gocron.NewScheduler(time.Local),
		sem:              make(chan struct{}, maxConcurrent),
		ctx:              ctx,
		cancel:           cancel,
		activeJobs:       make(map[int]*ActiveJob),
		nowFunc:          time.Now,
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

// ComputeNextStartTime returns next start time and overdue flag for a job (exported for testing).
func ComputeNextStartTime(jobItem *Job) (time.Time, bool) {
	return computeNextStartTime(jobItem)
}

// scheduleAt sets up the cron scheduler to trigger the job execution with concurrency control
func (s *Scheduler) scheduleAt(jobItem *Job, startAt time.Time, intervalSeconds int) {
	var err error
	if jobItem.Schedule != "" {
		// Use Cron schedule
		_, err = s.cronScheduler.Cron(jobItem.Schedule).
			StartAt(startAt).
			Do(func() { s.semWrapperWithContext(s.ctx, jobItem) })
		if err != nil {
			s.logger.Error("Failed to schedule job with cron schedule", zap.Int("jobID", jobItem.ID), zap.String("schedule", jobItem.Schedule), zap.Error(err))
		}
	} else {
		// Fallback to interval seconds
		_, err = s.cronScheduler.
			Every(intervalSeconds).Seconds().
			StartAt(startAt).
			Do(func() { s.semWrapperWithContext(s.ctx, jobItem) })
		if err != nil {
			s.logger.Error("Failed to schedule job with interval", zap.Int("jobID", jobItem.ID), zap.Int("interval", intervalSeconds), zap.Error(err))
		}
	}
}

// refreshJobs loads all enabled jobs, clears existing schedules, and reschedules each job
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

		nextStart, overdue := computeNextStartTime(&jobItem)
		s.logger.Info("Scheduling job", zap.Int("jobID", jobItem.ID), zap.Time("nextStart", nextStart), zap.Int("intervalSeconds", jobItem.Interval))

		s.scheduleAt(&jobItem, nextStart, jobItem.Interval)

		if overdue {
			s.logger.Info("Job is overdue, executing immediately", zap.Int("jobID", jobItem.ID))
			go s.semWrapperWithContext(s.ctx, &jobItem)
		}
	}
}

// Refresh reloads all enabled jobs, clearing and re-adding their schedules
func (s *Scheduler) Refresh() {
	s.cronScheduler.Clear()
	s.refreshJobs()
}

// ScheduleJob schedules a newly created or updated job, handling immediate execution if overdue
func (s *Scheduler) ScheduleJob(jobItem *Job) {
	if !jobItem.Enabled {
		return
	}

	nextStart, overdue := computeNextStartTime(jobItem)
	if overdue {
		s.logger.Info("New job is overdue, executing immediately", zap.Int("jobID", jobItem.ID))
		go s.semWrapperWithContext(s.ctx, jobItem)
		// Recurring schedule starts after interval
		newStart := time.Now().Add(time.Second * time.Duration(jobItem.Interval))
		s.logger.Info("Scheduling recurring job", zap.Int("jobID", jobItem.ID), zap.Time("newStart", newStart))
		s.scheduleAt(jobItem, newStart, jobItem.Interval)
	} else {
		s.logger.Info("Scheduling new job", zap.Int("jobID", jobItem.ID), zap.Time("nextStart", nextStart))
		s.scheduleAt(jobItem, nextStart, jobItem.Interval)
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
	// Render templated URL if defined
	downloadURL := jobItem.URL
	if strings.Contains(jobItem.URL, "{{") {
		tpl, err := template.New("url").Parse(jobItem.URL)
		if err != nil {
			s.logger.Error("URL template parse error", zap.Error(err))
		} else {
			var buf bytes.Buffer
			tpl.Execute(&buf, map[string]interface{}{"Now": time.Now()})
			downloadURL = buf.String()
		}
	}
	s.logger.Info("Executing job", zap.Int("jobID", jobItem.ID), zap.String("url", downloadURL))

	// Metrics: track active jobs
	activeJobsGauge.Inc()
	defer activeJobsGauge.Dec()

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

	// Download with ETag/Last-Modified and Range support
	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	hash, newETag, newLastMod, dlErr := download.DownloadFileExtended(downloadCtx, downloadURL, dest, jobItem.Overwrite, jobItem.LastETag, jobItem.LastModified, s.logger)
	// Handle 304 Not Modified
	if dlErr == download.ErrNotModified {
		s.logger.Info("Not modified - skipped ingest", zap.Int("jobID", jobItem.ID))
		// Update LastRun timestamp
		jobItem.LastRun = &startTime
		UpdateJob(s.db, *jobItem)
		// Mark job completed
		s.jobsMutex.Lock()
		s.activeJobs[jobItem.ID].Status = StatusCompleted
		s.jobsMutex.Unlock()
		return
	}
	if dlErr != nil {
		s.logger.Error("Download failed", zap.Int("jobID", jobItem.ID), zap.Error(dlErr))
		execErr = dlErr
	} else {
		fileHash = hash
		// Update ETag and Last-Modified for future requests
		jobItem.LastETag = newETag
		jobItem.LastModified = newLastMod
	}

	duration := time.Since(startTime)
	// Metrics: observe duration
	downloadDuration.Observe(duration.Seconds())
	if execErr != nil {
		s.logger.Error("Download failed", zap.Int("jobID", jobItem.ID), zap.Error(execErr), zap.Duration("duration", duration))
		downloadTotal.WithLabelValues("fail").Inc()
		s.jobsMutex.Lock()
		s.activeJobs[jobItem.ID].Status = StatusFailed
		s.activeJobs[jobItem.ID].Error = execErr
		s.jobsMutex.Unlock()
		return
	}

	s.logger.Info("Download succeeded", zap.Int("jobID", jobItem.ID), zap.Duration("duration", duration))
	downloadTotal.WithLabelValues("success").Inc()

	// Duplicate suppression: skip if hash already logged
	var exists bool
	_ = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM downloads WHERE job_id = ? AND file_hash = ?)`, jobItem.ID, fileHash).Scan(&exists)
	if exists {
		s.logger.Info("duplicate detected - skipped ingest", zap.Int("jobID", jobItem.ID), zap.String("hash", fileHash))
		// Persist updated headers and timestamp even on duplicate
		jobItem.LastETag = newETag
		jobItem.LastModified = newLastMod
		jobItem.LastRun = &startTime
		if err := UpdateJob(s.db, *jobItem); err != nil {
			s.logger.Error("Failed to update job after duplicate", zap.Int("jobID", jobItem.ID), zap.Error(err))
		}
		s.jobsMutex.Lock()
		s.activeJobs[jobItem.ID].Status = StatusCompleted
		s.jobsMutex.Unlock()
		return
	}

	if err := LogDownload(s.db, jobItem.ID, dest, fileHash); err != nil {
		s.logger.Error("Failed to log download", zap.Int("jobID", jobItem.ID), zap.Error(err))
	}

	// Post-download hook or emit
	now := time.Now()
	data := map[string]interface{}{"Path": dest, "Now": now}
	if jobItem.HookTemplate != "" {
		if !s.allowUnsafeHooks {
			s.logger.Error("Unsafe hooks disabled, skipping execution", zap.Int("jobID", jobItem.ID))
		} else {
			tpl, err := template.New("hook").Parse(jobItem.HookTemplate)
			if err != nil {
				s.logger.Error("hook template parse error", zap.Error(err))
			} else {
				var buf bytes.Buffer
				tpl.Execute(&buf, data)
				cmd := exec.CommandContext(ctx, "sh", "-c", buf.String())
				out, err := cmd.CombinedOutput()
				if err != nil {
					s.logger.Error("hook execution error", zap.Error(err), zap.ByteString("output", out))
				} else {
					s.logger.Info("hook executed successfully", zap.ByteString("output", out))
				}
			}
		}
	} else if jobItem.EmitTemplate != "" {
		tpl, err := template.New("emit").Parse(jobItem.EmitTemplate)
		if err != nil {
			s.logger.Error("emit template parse error", zap.Error(err))
		} else {
			var buf bytes.Buffer
			tpl.Execute(&buf, data)
			s.logger.Info("emit event", zap.String("message", buf.String()))
		}
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
	now := s.nowFunc()
	// Parse URL path
	parsed, err := url.Parse(jobItem.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	// Determine sub-directory
	var subdir string
	if jobItem.SubdirTemplate != "" {
		tpl, err := template.New("subdir").Parse(jobItem.SubdirTemplate)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		tpl.Execute(&buf, map[string]interface{}{"Now": now})
		subdir = buf.String()
	} else {
		subdir = fmt.Sprintf("job_%d", jobItem.ID)
	}
	jobDir := filepath.Join(s.downloadRoot, subdir)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return "", err
	}
	// Determine filename
	var filename string
	if jobItem.NameTemplate != "" {
		tpl, err := template.New("name").Parse(jobItem.NameTemplate)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		tpl.Execute(&buf, map[string]interface{}{"Now": now, "URL": jobItem.URL})
		filename = buf.String()
	} else {
		filename = path.Base(parsed.Path)
		if filename == "" {
			filename = "downloaded_file"
		}
		filename = util.EnsureSafeFilename(filename)
	}
	return filepath.Join(jobDir, filename), nil
}

// PrepareDestination constructs the download path for a job (exported for testing)
func (s *Scheduler) PrepareDestination(jobItem *Job) (string, error) {
	return s.prepareDestination(jobItem)
}

// SetNowFunc sets the function used to obtain current time (for testing)
func (s *Scheduler) SetNowFunc(fn func() time.Time) {
	s.nowFunc = fn
}
