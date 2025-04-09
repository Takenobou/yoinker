package job

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/Takenobou/yoinker/internal/download"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

const (
	maxRetries = 3
	retryDelay = 10 * time.Second
)

type Scheduler struct {
	db            *sql.DB
	logger        *zap.Logger
	downloadRoot  string
	maxConcurrent int
	cronScheduler *gocron.Scheduler
	sem           chan struct{}
}

func NewScheduler(db *sql.DB, logger *zap.Logger, downloadRoot string, maxConcurrent int) *Scheduler {
	return &Scheduler{
		db:            db,
		logger:        logger,
		downloadRoot:  downloadRoot,
		maxConcurrent: maxConcurrent,
		cronScheduler: gocron.NewScheduler(time.Local),
		sem:           make(chan struct{}, maxConcurrent),
	}
}

func (s *Scheduler) Start() {
	s.logger.Info("Scheduler starting with gocron")
	s.refreshJobs()
	s.cronScheduler.StartAsync()
}

func (s *Scheduler) Stop() {
	s.logger.Info("Scheduler stopping")
	s.cronScheduler.Stop()
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

		// Schedule recurring execution starting at nextStart.
		_, err := s.cronScheduler.
			Every(jobToSchedule.Interval).Seconds().
			StartAt(nextStart).
			Do(func() {
				s.sem <- struct{}{}
				s.executeJob(&jobToSchedule)
				<-s.sem
			})
		if err != nil {
			s.logger.Error("Failed to schedule job", zap.Int("jobID", jobToSchedule.ID), zap.Error(err))
		}

		// If overdue, trigger an immediate execution in a separate goroutine.
		if overdue {
			s.logger.Info("Job is overdue, executing immediately", zap.Int("jobID", jobToSchedule.ID))
			go s.semWrapper(&jobToSchedule)
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
		go s.semWrapper(jobItem)
		newStart := time.Now().Add(time.Second * time.Duration(jobItem.Interval))
		s.logger.Info("Scheduling recurring job", zap.Int("jobID", jobItem.ID), zap.Time("newStart", newStart))
		_, err := s.cronScheduler.
			Every(jobItem.Interval).Seconds().
			StartAt(newStart).
			Do(func() {
				s.sem <- struct{}{}
				s.executeJob(jobItem)
				<-s.sem
			})
		if err != nil {
			s.logger.Error("Failed to schedule recurring job", zap.Int("jobID", jobItem.ID), zap.Error(err))
		}
	} else {
		s.logger.Info("Scheduling new job", zap.Int("jobID", jobItem.ID), zap.Time("nextStart", nextStart))
		_, err := s.cronScheduler.
			Every(jobItem.Interval).Seconds().
			StartAt(nextStart).
			Do(func() {
				s.sem <- struct{}{}
				s.executeJob(jobItem)
				<-s.sem
			})
		if err != nil {
			s.logger.Error("Failed to schedule new job", zap.Int("jobID", jobItem.ID), zap.Error(err))
		}
	}
}

func (s *Scheduler) semWrapper(jobItem *Job) {
	s.sem <- struct{}{}
	s.executeJob(jobItem)
	<-s.sem
}

// executeJob attempts to download the job's file (with retries),
// logs the download, and updates the job's LastRun timestamp.
func (s *Scheduler) executeJob(jobItem *Job) {
	s.logger.Info("Executing job", zap.Int("jobID", jobItem.ID), zap.String("url", jobItem.URL))

	parsed, err := url.Parse(jobItem.URL)
	if err != nil {
		s.logger.Error("Failed to parse URL", zap.String("url", jobItem.URL), zap.Error(err))
		return
	}

	jobDir := filepath.Join(s.downloadRoot, fmt.Sprintf("job_%d", jobItem.ID))
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		s.logger.Error("Failed to create job directory", zap.String("jobDir", jobDir), zap.Error(err))
		return
	}

	filename := path.Base(parsed.Path)
	if filename == "" {
		filename = "downloaded_file"
	}
	dest := filepath.Join(jobDir, filename)

	var fileHash string
	var attempt int
	var execErr error
	startTime := time.Now()
	for attempt = 1; attempt <= maxRetries; attempt++ {
		s.logger.Info("Attempting download", zap.Int("attempt", attempt), zap.Int("jobID", jobItem.ID))
		fileHash, execErr = download.DownloadFile(jobItem.URL, dest, jobItem.Overwrite, s.logger)
		if execErr == nil {
			break
		}
		s.logger.Warn("Download attempt failed", zap.Int("attempt", attempt), zap.Int("jobID", jobItem.ID), zap.Error(execErr))
		time.Sleep(retryDelay)
	}
	if execErr != nil {
		s.logger.Error("All download attempts failed", zap.Int("jobID", jobItem.ID), zap.Error(execErr))
		return
	}

	duration := time.Since(startTime)
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
}
