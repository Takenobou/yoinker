package test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
