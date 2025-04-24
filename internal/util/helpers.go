package util

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ComputeFileMD5 computes the MD5 hash of the file at the specified path.
func ComputeFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// FilesEqual compares the MD5 hash of two files to determine if they are identical.
func FilesEqual(filePath1, filePath2 string) (bool, error) {
	hash1, err := ComputeFileMD5(filePath1)
	if err != nil {
		return false, err
	}

	hash2, err := ComputeFileMD5(filePath2)
	if err != nil {
		return false, err
	}

	return hash1 == hash2, nil
}

// ValidateURL checks if the provided URL is valid and has a supported scheme.
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check if the URL has a scheme
	if u.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http:// or https://)")
	}

	// Validate scheme is supported
	supportedSchemes := []string{"http", "https", "ftp"}
	schemeValid := false
	for _, scheme := range supportedSchemes {
		if u.Scheme == scheme {
			schemeValid = true
			break
		}
	}

	if !schemeValid {
		return fmt.Errorf("URL scheme '%s' is not supported. Use: %s",
			u.Scheme, strings.Join(supportedSchemes, ", "))
	}

	// Check if the URL has a host
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// ValidateInterval checks if the provided interval is valid.
func ValidateInterval(interval int) error {
	// Minimum interval is 10 seconds to prevent too frequent downloads
	const minInterval = 10

	if interval < minInterval {
		return fmt.Errorf("interval must be at least %d seconds", minInterval)
	}

	return nil
}

// EnsureSafeFilename ensures the filename doesn't contain dangerous characters
func EnsureSafeFilename(filename string) string {
	// Replace potentially dangerous characters
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", ";", "&"}
	safe := filename

	for _, char := range unsafe {
		safe = strings.ReplaceAll(safe, char, "_")
	}

	return safe
}

// GetEnv returns the environment variable key or defaultVal if unset
func GetEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// GetEnvAsInt returns the environment variable key as int or defaultVal if unset or invalid
func GetEnvAsInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

// GetRequiredEnv returns the environment variable key or errors if unset
func GetRequiredEnv(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s environment variable required", key)
}
