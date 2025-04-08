package storage

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Takenobou/yoinker/internal/app"
)

func InitStorage(cfg *app.Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
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
			last_run TIMESTAMP
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
