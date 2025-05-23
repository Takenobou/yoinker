package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Takenobou/yoinker/internal/util"
)

type Config struct {
	Port                   string
	DBPath                 string
	LogLevel               string
	DownloadRoot           string
	MaxConcurrentDownloads int
	AllowUnsafeHooks       bool
}

func LoadConfig() (*Config, error) {
	dbPath, exists := os.LookupEnv("DB_PATH")
	if !exists || dbPath == "" {
		return nil, fmt.Errorf("DB_PATH must be set")
	}

	cfg := &Config{
		Port:                   util.GetEnv("PORT", "3000"),
		DBPath:                 dbPath,
		LogLevel:               util.GetEnv("LOG_LEVEL", "info"),
		DownloadRoot:           util.GetEnv("DOWNLOAD_ROOT", "downloads"),
		MaxConcurrentDownloads: util.GetEnvAsInt("MAX_CONCURRENT_DOWNLOADS", 5),
		AllowUnsafeHooks:       util.GetEnvAsBool("HOOK_UNSAFE", false),
	}

	// If DBPath refers to a directory, append default DB filename
	if info, err := os.Stat(cfg.DBPath); err == nil && info.IsDir() {
		cfg.DBPath = filepath.Join(cfg.DBPath, "yoinker.db")
	}
	// Ensure parent directory for the database file exists
	dbDir := filepath.Dir(cfg.DBPath)
	if dbDir != "." {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create DB directory: %w", err)
		}
	}

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateConfig ensures all config values are valid
func validateConfig(cfg *Config) error {
	// Validate Port
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return fmt.Errorf("invalid PORT value: %s", cfg.Port)
	}

	// Validate MaxConcurrentDownloads
	if cfg.MaxConcurrentDownloads <= 0 {
		return fmt.Errorf("MAX_CONCURRENT_DOWNLOADS must be greater than 0")
	}

	// Ensure download directory exists or can be created
	if _, err := os.Stat(cfg.DownloadRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.DownloadRoot, 0755); err != nil {
			return fmt.Errorf("cannot create download directory: %w", err)
		}
	}

	return nil
}
