package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Takenobou/yoinker/internal/job"
	"github.com/Takenobou/yoinker/internal/web"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type Server struct {
	cfg       *Config
	fiberApp  *fiber.App
	db        *sql.DB
	logger    *zap.Logger
	scheduler *job.Scheduler
}

func NewServer(cfg *Config, db *sql.DB, logger *zap.Logger, scheduler *job.Scheduler) *Server {
	appFiber := fiber.New()
	s := &Server{
		cfg:       cfg,
		fiberApp:  appFiber,
		db:        db,
		logger:    logger,
		scheduler: scheduler,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.fiberApp.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	s.fiberApp.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Welcome to Yoinker")
	})

	handlers := web.NewHandlers(s.db, s.logger, s.scheduler)
	handlers.RegisterRoutes(s.fiberApp)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	s.logger.Info("Starting server", zap.String("address", addr))
	return s.fiberApp.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server")
	return s.fiberApp.Shutdown()
}
