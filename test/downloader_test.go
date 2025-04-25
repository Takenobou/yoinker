package test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Takenobou/yoinker/internal/download"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestTimestampedFilename(t *testing.T) {
	orig := "/tmp/file.txt"
	ts1 := download.TimestampedFilename(orig)
	time.Sleep(1 * time.Second)
	ts2 := download.TimestampedFilename(orig)
	if ts1 == ts2 {
		t.Error("Expected two timestamped filenames to differ")
	}
	if filepath.Ext(ts1) != ".txt" {
		t.Errorf("Expected extension .txt, got %s", filepath.Ext(ts1))
	}
}

func TestDownloadFileWithContext(t *testing.T) {
	// create a test server returning fixed content
	content := []byte("test content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	tmp, err := os.MkdirTemp("", "dltest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	dest := filepath.Join(tmp, "out.txt")
	// use a logger ignoring output
	logger, _ := zap.NewProduction(zap.IncreaseLevel(zapcore.DebugLevel))
	hash, err := download.DownloadFileWithContext(context.Background(), srv.URL, dest, true, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// compute expected hash
	h := md5.New()
	h.Write(content)
	expected := hex.EncodeToString(h.Sum(nil))
	if hash != expected {
		t.Errorf("Expected hash %s, got %s", expected, hash)
	}
	// file should exist and content match
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("File content mismatch, expected %q, got %q", content, data)
	}
}

// TestDownloadResumable verifies that partial .download files are resumed via Range header
func TestDownloadResumable(t *testing.T) {
	content := []byte("HelloWorld")
	var mu sync.Mutex
	var receivedRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		rng := r.Header.Get("Range")
		receivedRange = rng
		if rng != "" {
			parts := strings.Split(rng, "=")
			offset, _ := strconv.Atoi(strings.TrimSuffix(parts[1], "-"))
			w.Write(content[offset:])
		} else {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Write(content)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.txt")
	// simulate partial download: write first half
	if err := os.WriteFile(dest+".download", content[:5], 0644); err != nil {
		t.Fatal(err)
	}
	logger, _ := zap.NewProduction(zap.IncreaseLevel(zapcore.DebugLevel))
	_, err := download.DownloadFileWithContext(context.Background(), srv.URL, dest, true, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("Expected full content %q, got %q", content, data)
	}
	mu.Lock()
	defer mu.Unlock()
	if receivedRange != "bytes=5-" {
		t.Errorf("Expected Range header bytes=5-, got %s", receivedRange)
	}
}
