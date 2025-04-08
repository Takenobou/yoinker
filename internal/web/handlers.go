package web

import (
	"database/sql"
	"strconv"

	"github.com/Takenobou/yoinker/internal/job"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type Handlers struct {
	DB     *sql.DB
	Logger *zap.Logger
}

func NewHandlers(db *sql.DB, logger *zap.Logger) *Handlers {
	return &Handlers{
		DB:     db,
		Logger: logger,
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

// ListJobs returns all jobs from the database
func (h *Handlers) ListJobs(c *fiber.Ctx) error {
	jobs, err := job.ListJobs(h.DB)
	if err != nil {
		h.Logger.Error("Failed to list jobs", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(jobs)
}

// GetJob returns a single job by its ID
func (h *Handlers) GetJob(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.Logger.Error("Invalid job ID", zap.String("id", idStr), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID"})
	}
	jobData, err := job.GetJob(h.DB, id)
	if err != nil {
		h.Logger.Error("Failed to get job", zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found"})
	}
	return c.JSON(jobData)
}

// CreateJob creates a new job
func (h *Handlers) CreateJob(c *fiber.Ctx) error {
	var jobData job.Job
	if err := c.BodyParser(&jobData); err != nil {
		h.Logger.Error("Failed to parse job data", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job data"})
	}
	id, err := job.CreateJob(h.DB, jobData)
	if err != nil {
		h.Logger.Error("Failed to create job", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create job"})
	}
	return c.JSON(fiber.Map{"id": id})
}

// UpdateJob updates an existing job
func (h *Handlers) UpdateJob(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.Logger.Error("Invalid job ID", zap.String("id", idStr), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID"})
	}
	var jobData job.Job
	if err := c.BodyParser(&jobData); err != nil {
		h.Logger.Error("Failed to parse job data", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job data"})
	}
	jobData.ID = id
	if err := job.UpdateJob(h.DB, jobData); err != nil {
		h.Logger.Error("Failed to update job", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update job"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

// DeleteJob deletes a job by its ID
func (h *Handlers) DeleteJob(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.Logger.Error("Invalid job ID", zap.String("id", idStr), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID"})
	}
	if err := job.DeleteJob(h.DB, id); err != nil {
		h.Logger.Error("Failed to delete job", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete job"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}
