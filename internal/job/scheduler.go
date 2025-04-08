package job

import (
	"database/sql"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/Takenobou/yoinker/internal/download"
	"go.uber.org/zap"
)

type Scheduler struct {
	db     *sql.DB
	logger *zap.Logger
	stop   chan struct{}
}

func NewScheduler(db *sql.DB, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		db:     db,
		logger: logger,
		stop:   make(chan struct{}),
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

	if err := os.MkdirAll("downloads", 0755); err != nil {
		s.logger.Error("Failed to create downloads directory", zap.Error(err))
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

	now := time.Now()
	for _, jobItem := range jobs {
		if jobItem.LastRun == nil || now.Sub(*jobItem.LastRun) >= time.Duration(jobItem.Interval)*time.Minute {
			s.logger.Info("Executing job", zap.Int("jobID", jobItem.ID), zap.String("url", jobItem.URL))

			parsed, err := url.Parse(jobItem.URL)
			if err != nil {
				s.logger.Error("Failed to parse URL", zap.String("url", jobItem.URL), zap.Error(err))
				continue
			}
			filename := path.Base(parsed.Path)
			if filename == "" {
				filename = "downloaded_file"
			}
			dest := "downloads/" + filename

			err = download.DownloadFile(jobItem.URL, dest, jobItem.Overwrite, s.logger)
			if err != nil {
				s.logger.Error("Download failed", zap.Int("jobID", jobItem.ID), zap.Error(err))
				continue
			}

			jobItem.LastRun = &now
			if err := UpdateJob(s.db, jobItem); err != nil {
				s.logger.Error("Failed to update job last_run", zap.Int("jobID", jobItem.ID), zap.Error(err))
			} else {
				s.logger.Info("Job execution updated", zap.Int("jobID", jobItem.ID))
			}
		}
	}
}
