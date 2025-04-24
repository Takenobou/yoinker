package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Takenobou/yoinker/internal/config"
)

func TestLoadConfigMissingDBPath(t *testing.T) {
	// Ensure DB_PATH unset
	os.Unsetenv("DB_PATH")
	_, err := config.LoadConfig()
	if err == nil {
		t.Fatal("Expected error when DB_PATH is missing")
	}
}

func TestLoadConfigInvalidPort(t *testing.T) {
	os.Setenv("DB_PATH", "test.db")
	os.Setenv("PORT", "notanumber")
	_, err := config.LoadConfig()
	if err == nil || !contains(err.Error(), "invalid PORT") {
		t.Fatalf("Expected invalid PORT error, got %v", err)
	}
}

func TestLoadConfigCreatesDownloadRoot(t *testing.T) {
	tmp := os.TempDir()
	downloadRoot := filepath.Join(tmp, "yoinker_test_downloads")
	// Clean up
	os.RemoveAll(downloadRoot)

	os.Setenv("DB_PATH", "test.db")
	os.Setenv("PORT", "8080")
	os.Setenv("DOWNLOAD_ROOT", downloadRoot)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.DownloadRoot != downloadRoot {
		t.Errorf("Expected DownloadRoot %q, got %q", downloadRoot, cfg.DownloadRoot)
	}
	// Directory should have been created
	if info, err := os.Stat(downloadRoot); err != nil || !info.IsDir() {
		t.Errorf("Expected download root directory to exist, err=%v", err)
	}
	// Clean up
	os.RemoveAll(downloadRoot)
}

func contains(s, substr string) bool {
	return filepath.Base(s) == substr || len(s) >= len(substr) && s[:len(substr)] == substr
}
