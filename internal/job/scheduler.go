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
	stop          chan struct{}
}

func NewScheduler(db *sql.DB, logger *zap.Logger, downloadRoot string, maxConcurrent int) *Scheduler {
	return &Scheduler{
		db:            db,
		logger:        logger,
		downloadRoot:  downloadRoot,
		maxConcurrent: maxConcurrent,
		stop:          make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.logger.Info("Scheduler started")
	go s.run()
}

func (s *Scheduler) Stop() {
	s.logger.Info("Scheduler stopping")
	close(s.stop)
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	if err := os.MkdirAll(s.downloadRoot, 0755); err != nil {
		s.logger.Error("Failed to create download root directory", zap.Error(err))
	}

	for {
		select {
		case <-ticker.C:
			s.runJobs()
		case <-s.stop:
			s.logger.Info("Scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) runJobs() {
	s.logger.Info("Running scheduled jobs")

	jobs, err := ListJobs(s.db)
	if err != nil {
		s.logger.Error("Failed to list jobs", zap.Error(err))
		return
	}

	sem := make(chan struct{}, s.maxConcurrent)
	now := time.Now()
	for _, jobItem := range jobs {
		if !jobItem.Enabled {
			continue
		}

		if jobItem.LastRun == nil || now.Sub(*jobItem.LastRun) >= time.Duration(jobItem.Interval)*time.Minute {
			sem <- struct{}{}
			go func(jobItem Job) {
				defer func() { <-sem }()
				s.executeJob(&jobItem)
			}(jobItem)
		}
	}

	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
}

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

	jobItem.LastRun = &startTime
	if err := UpdateJob(s.db, *jobItem); err != nil {
		s.logger.Error("Failed to update job last_run", zap.Int("jobID", jobItem.ID), zap.Error(err))
	} else {
		s.logger.Info("Job execution updated", zap.Int("jobID", jobItem.ID))
	}
}
