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
}

func CreateJob(db *sql.DB, job Job) (int64, error) {
	result, err := db.Exec(`INSERT INTO jobs (url, interval, overwrite, last_run) VALUES (?, ?, ?, ?)`, job.URL, job.Interval, job.Overwrite, job.LastRun)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetJob(db *sql.DB, id int) (*Job, error) {
	var job Job
	var lastRun sql.NullTime
	err := db.QueryRow(`SELECT id, url, interval, overwrite, last_run FROM jobs WHERE id = ?`, id).Scan(&job.ID, &job.URL, &job.Interval, &job.Overwrite, &lastRun)
	if err != nil {
		return nil, err
	}
	if lastRun.Valid {
		job.LastRun = &lastRun.Time
	}
	return &job, nil
}

func ListJobs(db *sql.DB) ([]Job, error) {
	rows, err := db.Query(`SELECT id, url, interval, overwrite, last_run FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var lastRun sql.NullTime
		if err := rows.Scan(&job.ID, &job.URL, &job.Interval, &job.Overwrite, &lastRun); err != nil {
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
	_, err := db.Exec(`UPDATE jobs SET url = ?, interval = ?, overwrite = ?, last_run = ? WHERE id = ?`, job.URL, job.Interval, job.Overwrite, job.LastRun, job.ID)
	return err
}

func DeleteJob(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}
