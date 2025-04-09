package app

import (
	"os"
	"strconv"
)

type Config struct {
	Port                   string
	DBPath                 string
	LogLevel               string
	DownloadRoot           string
	MaxConcurrentDownloads int
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                   getEnv("PORT", "3000"),
		DBPath:                 getEnv("DB_PATH", "yoinker.db"),
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		DownloadRoot:           getEnv("DOWNLOAD_ROOT", "downloads"),
		MaxConcurrentDownloads: getEnvAsInt("MAX_CONCURRENT_DOWNLOADS", 5),
	}
	return cfg, nil
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
