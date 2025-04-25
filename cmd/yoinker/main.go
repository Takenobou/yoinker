package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Takenobou/yoinker/internal/app"
	"github.com/Takenobou/yoinker/internal/config"
	"github.com/Takenobou/yoinker/internal/job"
	"github.com/Takenobou/yoinker/internal/storage"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) > 1 {
		handleCLI(os.Args[1:])
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	logger, err := app.InitLogger(cfg)
	if err != nil {
		log.Fatalf("Error initialising logger: %v", err)
	}
	logger.Info("Starting yoinker")

	// Add system info to startup log
	logger.Info("System information",
		zap.Int("numCPU", runtime.NumCPU()),
		zap.String("goVersion", runtime.Version()),
		zap.String("osType", runtime.GOOS),
		zap.String("architecture", runtime.GOARCH),
	)

	db, err := storage.InitStorage(cfg)
	if err != nil {
		logger.Fatal("Error initialising database", zap.Error(err))
	}
	defer db.Close()

	scheduler := job.NewScheduler(db, logger, cfg.DownloadRoot, cfg.MaxConcurrentDownloads)
	scheduler.Start()
	defer scheduler.Stop()

	server := app.NewServer(cfg, db, logger, scheduler)

	// Register enhanced health and metrics endpoints
	server.RegisterHealthChecks(db)

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

// handleCLI processes command-line subcommands: add, ls, rm, stats, apply
func handleCLI(args []string) {
	cmd := args[0]
	// Load config for DB path
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Config error:", err)
		os.Exit(1)
	}
	db, err := storage.InitStorage(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "DB error:", err)
		os.Exit(1)
	}
	defer db.Close()
	switch cmd {
	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		url := fs.String("url", "", "Download URL")
		interval := fs.Int("interval", 60, "Interval seconds")
		schedule := fs.String("schedule", "", "Cron schedule")
		overwrite := fs.Bool("overwrite", false, "Overwrite existing files")
		fs.Parse(args[1:])
		jobID, err := job.CreateJob(db, job.Job{URL: *url, Interval: *interval, Schedule: *schedule, Overwrite: *overwrite, Enabled: true})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Add job error:", err)
			os.Exit(1)
		}
		fmt.Println("Added job ID", jobID)
	case "ls":
		jobs, err := job.ListJobs(db)
		if err != nil {
			fmt.Fprintln(os.Stderr, "List jobs error:", err)
			os.Exit(1)
		}
		data, _ := json.MarshalIndent(jobs, "", "  ")
		fmt.Println(string(data))
	case "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: yoinker rm <id>")
			os.Exit(1)
		}
		id := args[1]
		// parse id
		var jobID int
		fmt.Sscan(id, &jobID)
		if err := job.DeleteJob(db, jobID); err != nil {
			fmt.Fprintln(os.Stderr, "Delete job error:", err)
			os.Exit(1)
		}
		fmt.Println("Deleted job ID", jobID)
	case "stats":
		jobs, _ := job.ListJobs(db)
		var dlCount int
		db.QueryRow("SELECT COUNT(*) FROM downloads").Scan(&dlCount)
		fmt.Printf("Jobs: %d, Downloads: %d\n", len(jobs), dlCount)
	case "apply":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: yoinker apply <config.yml>")
			os.Exit(1)
		}
		data, err := ioutil.ReadFile(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Read config error:", err)
			os.Exit(1)
		}
		var jobs []job.Job
		if err := yaml.Unmarshal(data, &jobs); err != nil {
			fmt.Fprintln(os.Stderr, "YAML parse error:", err)
			os.Exit(1)
		}
		for _, j := range jobs {
			_, err := job.CreateJob(db, j)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Create job error for URL", j.URL, err)
			}
			fmt.Println("Applied job URL", j.URL)
		}
	default:
		fmt.Fprintln(os.Stderr, "Unknown command:", cmd)
		os.Exit(1)
	}
}
