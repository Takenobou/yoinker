package job

import (
	"database/sql"
	"time"
)

type Job struct {
	ID        int
	URL       string
	Interval  int
	Overwrite bool
	LastRun   *time.Time
	Enabled   bool
}

func CreateJob(db *sql.DB, job Job) (int64, error) {
	result, err := db.Exec(`INSERT INTO jobs (url, interval, overwrite, last_run, enabled) VALUES (?, ?, ?, ?, ?)`,
		job.URL, job.Interval, job.Overwrite, job.LastRun, job.Enabled)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetJob(db *sql.DB, id int) (*Job, error) {
	var job Job
	var lastRun sql.NullTime
	err := db.QueryRow(`SELECT id, url, interval, overwrite, last_run, enabled FROM jobs WHERE id = ?`,
		id).Scan(&job.ID, &job.URL, &job.Interval, &job.Overwrite, &lastRun, &job.Enabled)
	if err != nil {
		return nil, err
	}
	if lastRun.Valid {
		job.LastRun = &lastRun.Time
	}
	return &job, nil
}

func ListJobs(db *sql.DB) ([]Job, error) {
	rows, err := db.Query(`SELECT id, url, interval, overwrite, last_run, enabled FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var lastRun sql.NullTime
		if err := rows.Scan(&job.ID, &job.URL, &job.Interval, &job.Overwrite, &lastRun, &job.Enabled); err != nil {
			return nil, err
		}
		if lastRun.Valid {
			job.LastRun = &lastRun.Time
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func UpdateJob(db *sql.DB, job Job) error {
	_, err := db.Exec(`UPDATE jobs SET url = ?, interval = ?, overwrite = ?, last_run = ?, enabled = ? WHERE id = ?`,
		job.URL, job.Interval, job.Overwrite, job.LastRun, job.Enabled, job.ID)
	return err
}

func DeleteJob(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func LogDownload(db *sql.DB, jobID int, filePath, fileHash string) error {
	_, err := db.Exec(`INSERT INTO downloads (job_id, file_path, downloaded_at, file_hash) VALUES (?, ?, ?, ?)`,
		jobID, filePath, time.Now(), fileHash)
	return err
}
