package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Takenobou/yoinker/internal/app"
	"github.com/Takenobou/yoinker/internal/job"
	"github.com/Takenobou/yoinker/internal/storage"
	"go.uber.org/zap"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	logger, err := app.InitLogger(cfg)
	if err != nil {
		log.Fatalf("Error initialising logger: %v", err)
	}
	logger.Info("Starting yoinker")

	db, err := storage.InitStorage(cfg)
	if err != nil {
		logger.Fatal("Error initialising database", zap.Error(err))
	}
	defer db.Close()

	scheduler := job.NewScheduler(db, logger, cfg.DownloadRoot, cfg.MaxConcurrentDownloads)
	scheduler.Start()
	defer scheduler.Stop()

	server := app.NewServer(cfg, db, logger, scheduler)
	go func() {
		if err := server.Start(); err != nil {
			logger.Fatal("Error starting server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down yoinker...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}
}
