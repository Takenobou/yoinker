package test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Takenobou/yoinker/internal/job"
	"go.uber.org/zap"
)

func TestComputeNextStartTime(t *testing.T) {
	now := time.Now()
	// Case: never run
	j1 := &job.Job{Interval: 60}
	next, overdue := job.ComputeNextStartTime(j1)
	if !overdue {
		t.Errorf("expected overdue for never-run job")
	}
	if next.Before(now) {
		t.Errorf("expected next after now, got %v", next)
	}

	// Case: last run sufficiently in past
	past := now.Add(-120 * time.Second)
	j2 := &job.Job{Interval: 30, LastRun: &past}
	next2, overdue2 := job.ComputeNextStartTime(j2)
	if !overdue2 {
		t.Errorf("expected overdue when candidate <= now")
	}
	// As candidate <= now, schedule interval after now
	if next2.Before(now) {
		t.Errorf("expected next2 after now, got %v", next2)
	}

	// Case: future scheduled run
	future := now.Add(120 * time.Second)
	j3 := &job.Job{Interval: 60, LastRun: &future}
	next3, overdue3 := job.ComputeNextStartTime(j3)
	if overdue3 {
		t.Errorf("expected not overdue for future lastRun")
	}
	if !next3.Equal(future.Add(60 * time.Second)) {
		t.Errorf("expected next = lastRun+interval, got %v", next3)
	}
}

func TestPrepareDestination_Templates(t *testing.T) {
	root := t.TempDir()
	logger := zap.NewNop()
	s := job.NewScheduler(nil, logger, root, 1, false)
	now := time.Date(2025, 4, 25, 10, 0, 0, 0, time.UTC)
	s.SetNowFunc(func() time.Time { return now })

	j := &job.Job{ID: 42, URL: "http://example.com/path/file.txt", NameTemplate: "file_{{.Now.Unix}}.txt", SubdirTemplate: "dir/{{.Now.Format \"2006-01-02\"}}"}
	dest, err := s.PrepareDestination(j)
	if err != nil {
		t.Fatal(err)
	}

	expectedDir := filepath.Join(root, "dir", now.Format("2006-01-02"))
	if filepath.Dir(dest) != expectedDir {
		t.Errorf("expected dir %s, got %s", expectedDir, filepath.Dir(dest))
	}
	expectedName := "file_" + fmt.Sprint(now.Unix()) + ".txt"
	if filepath.Base(dest) != expectedName {
		t.Errorf("expected filename %s, got %s", expectedName, filepath.Base(dest))
	}
}
