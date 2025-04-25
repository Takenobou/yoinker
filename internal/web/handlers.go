package web

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Takenobou/yoinker/internal/job"
	"github.com/Takenobou/yoinker/internal/util"
)

type Handlers struct {
	DB        *sql.DB
	Logger    *zap.Logger
	Scheduler *job.Scheduler
}

func NewHandlers(db *sql.DB, logger *zap.Logger, scheduler *job.Scheduler) *Handlers {
	return &Handlers{
		DB:        db,
		Logger:    logger,
		Scheduler: scheduler,
	}
}

func (h *Handlers) RegisterRoutes(app *fiber.App) {
	// Job management endpoints under /jobs
	group := app.Group("/jobs")
	group.Get("/", h.ListJobs)
	group.Get("/:id", h.GetJob)
	group.Post("/", h.CreateJob)
	group.Put("/:id", h.UpdateJob)
	group.Delete("/:id", h.DeleteJob)
}

// parseID extracts an integer path parameter and handles errors
func (h *Handlers) parseID(c *fiber.Ctx, name string) (int, bool) {
	id, err := ParamInt(c, name)
	if err != nil {
		h.Logger.Error("Invalid parameter", zap.String("param", name), zap.Error(err))
		respondError(c, fiber.StatusBadRequest, "Invalid "+name)
		return 0, false
	}
	return id, true
}

// parseJob parses and validates job data from the request body
func (h *Handlers) parseJob(c *fiber.Ctx) (*job.Job, bool) {
	var j job.Job
	if err := c.BodyParser(&j); err != nil {
		h.Logger.Error("Failed to parse job data", zap.Error(err))
		respondError(c, fiber.StatusBadRequest, "Invalid job data")
		return nil, false
	}
	if err := validateJobInput(&j, h.Logger); err != nil {
		respondError(c, fiber.StatusBadRequest, err.Error())
		return nil, false
	}
	return &j, true
}

// ListJobs returns all jobs from the database
func (h *Handlers) ListJobs(c *fiber.Ctx) error {
	jobs, err := job.ListJobs(h.DB)
	if err != nil {
		h.Logger.Error("Failed to list jobs", zap.Error(err))
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return respond(c, fiber.StatusOK, jobs)
}

// GetJob returns a single job by its ID
func (h *Handlers) GetJob(c *fiber.Ctx) error {
	id, ok := h.parseID(c, "id")
	if !ok {
		return nil
	}
	jobData, err := job.GetJob(h.DB, id)
	if err != nil {
		h.Logger.Error("Failed to get job", zap.Error(err))
		return respondError(c, fiber.StatusNotFound, "Job not found")
	}
	return respond(c, fiber.StatusOK, jobData)
}

// validateJobInput validates the job data for creation or updates
func validateJobInput(jobData *job.Job, logger *zap.Logger) error {
	// Validate URL
	if err := util.ValidateURL(jobData.URL); err != nil {
		logger.Error("Invalid URL", zap.String("url", jobData.URL), zap.Error(err))
		return err
	}

	// Validate interval if no schedule provided
	if jobData.Schedule == "" {
		if err := util.ValidateInterval(jobData.Interval); err != nil {
			logger.Error("Invalid interval", zap.Int("interval", jobData.Interval), zap.Error(err))
			return err
		}
	}

	// Validate schedule if provided
	if jobData.Schedule != "" {
		sched := gocron.NewScheduler(time.UTC)
		// Use Do on a dummy function to catch parsing errors
		_, err := sched.Cron(jobData.Schedule).Do(func() {})
		if err != nil {
			logger.Error("Invalid schedule", zap.String("schedule", jobData.Schedule), zap.Error(err))
			return fmt.Errorf("invalid schedule: %w", err)
		}
	}

	return nil
}

// CreateJob creates a new job, inserts it into the database, and immediately schedules it.
func (h *Handlers) CreateJob(c *fiber.Ctx) error {
	jobData, ok := h.parseJob(c)
	if !ok {
		return nil
	}

	// Set default values if needed
	if !jobData.Enabled {
		jobData.Enabled = true // Enable by default
	}

	id, err := job.CreateJob(h.DB, *jobData)
	if err != nil {
		h.Logger.Error("Failed to create job", zap.Error(err))
		return respondError(c, fiber.StatusInternalServerError, "Failed to create job")
	}

	// Populate the job with the assigned ID.
	jobData.ID = int(id)

	// Schedule the new job immediately.
	h.Scheduler.ScheduleJob(jobData)

	h.Logger.Info("Job created successfully",
		zap.Int("id", int(id)),
		zap.String("url", jobData.URL),
		zap.Int("interval", jobData.Interval))
	return respond(c, fiber.StatusCreated, fiber.Map{"id": id, "message": "Job created successfully"})
}

// UpdateJob updates an existing job
func (h *Handlers) UpdateJob(c *fiber.Ctx) error {
	id, ok := h.parseID(c, "id")
	if !ok {
		return nil
	}

	// First fetch the existing job to ensure it exists
	existingJob, err := job.GetJob(h.DB, id)
	if err != nil {
		h.Logger.Error("Job not found", zap.Error(err))
		return respondError(c, fiber.StatusNotFound, "Job not found")
	}

	jobData, ok := h.parseJob(c)
	if !ok {
		return nil
	}

	// Preserve the ID, LastRun, and Enabled fields from the existing job
	jobData.ID = id
	if jobData.LastRun == nil {
		jobData.LastRun = existingJob.LastRun
	}
	// If enabled not provided in update, keep existing setting
	jobData.Enabled = existingJob.Enabled

	if err := job.UpdateJob(h.DB, *jobData); err != nil {
		h.Logger.Error("Failed to update job", zap.Error(err))
		return respondError(c, fiber.StatusInternalServerError, "Failed to update job")
	}

	// If the job is enabled, reschedule it with the updated parameters
	if jobData.Enabled {
		h.Scheduler.ScheduleJob(jobData)
	}

	h.Logger.Info("Job updated successfully",
		zap.Int("id", id),
		zap.String("url", jobData.URL),
		zap.Int("interval", jobData.Interval),
		zap.Bool("enabled", jobData.Enabled))

	return respond(c, fiber.StatusOK, fiber.Map{"status": "updated", "message": "Job updated successfully"})
}

// DeleteJob deletes a job by its ID
func (h *Handlers) DeleteJob(c *fiber.Ctx) error {
	id, ok := h.parseID(c, "id")
	if !ok {
		return nil
	}

	// First check if the job exists
	_, err := job.GetJob(h.DB, id)
	if err != nil {
		h.Logger.Error("Job not found for deletion", zap.Error(err))
		return respondError(c, fiber.StatusNotFound, "Job not found")
	}

	if err := job.DeleteJob(h.DB, id); err != nil {
		h.Logger.Error("Failed to delete job", zap.Error(err))
		return respondError(c, fiber.StatusInternalServerError, "Failed to delete job")
	}

	h.Logger.Info("Job deleted successfully", zap.Int("id", id))
	return respond(c, fiber.StatusOK, fiber.Map{"status": "deleted", "message": "Job deleted successfully"})
}
