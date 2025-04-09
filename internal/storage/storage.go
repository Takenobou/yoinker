package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Takenobou/yoinker/internal/app"
	_ "github.com/mattn/go-sqlite3"
)

func InitStorage(cfg *app.Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Add a timeout for initialization operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		return nil, err
	}

	if err = runMigrations(db); err != nil {
		return nil, err
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			interval INTEGER NOT NULL,
			overwrite BOOLEAN NOT NULL,
			last_run TIMESTAMP,
			enabled BOOLEAN NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			downloaded_at TIMESTAMP NOT NULL,
			file_hash TEXT,
			FOREIGN KEY(job_id) REFERENCES jobs(id)
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}
