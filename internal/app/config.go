package app

import (
	"fmt"
	"os"
	"strconv"
)

// getRequiredEnv retrieves an environment variable or errors if unset
func getRequiredEnv(key string) (string, error) {
	if value := os.Getenv(key); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("%s environment variable required", key)
}

type Config struct {
	Port                   string
	DBPath                 string
	LogLevel               string
	DownloadRoot           string
	MaxConcurrentDownloads int
}

func LoadConfig() (*Config, error) {
	// Required DB_PATH
	dbPath, err := getRequiredEnv("DB_PATH")
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Port:                   getEnv("PORT", "3000"),
		DBPath:                 dbPath,
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		DownloadRoot:           getEnv("DOWNLOAD_ROOT", "downloads"),
		MaxConcurrentDownloads: getEnvAsInt("MAX_CONCURRENT_DOWNLOADS", 5),
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

func getEnv(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}

func getEnvAsInt(name string, defaultVal int) int {
	valueStr := os.Getenv(name)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultVal
}
