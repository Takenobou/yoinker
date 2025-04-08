package app

import "os"

type Config struct {
	Port     string
	DBPath   string
	LogLevel string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:     getEnv("PORT", "3000"),
		DBPath:   getEnv("DB_PATH", "yoinker.db"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
