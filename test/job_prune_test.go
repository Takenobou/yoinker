package test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/Takenobou/yoinker/internal/job"
	_ "github.com/mattn/go-sqlite3"
)

func TestPruneDownloads(t *testing.T) {
	// Setup in-memory DB and create downloads table
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create downloads table
	_, err = db.Exec(`CREATE TABLE downloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		file_path TEXT NOT NULL,
		downloaded_at TIMESTAMP NOT NULL,
		file_hash TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert old and recent records
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-12 * time.Hour)
	_, err = db.Exec(`INSERT INTO downloads (job_id, file_path, downloaded_at, file_hash) VALUES (?, ?, ?, ?), (?, ?, ?, ?)`,
		1, "old.txt", old, "h1",
		2, "recent.txt", recent, "h2",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Prune downloads older than 1 day
	cutoff := now.Add(-24 * time.Hour)
	count, err := job.PruneDownloads(db, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 record pruned, got %d", count)
	}

	// Ensure remaining record is the recent one
	var remaining int
	err = db.QueryRow(`SELECT COUNT(*) FROM downloads`).Scan(&remaining)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Errorf("expected 1 remaining record, got %d", remaining)
	}
}
