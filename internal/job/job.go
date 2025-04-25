package job

import (
	"database/sql"
	"time"
)

type Job struct {
	ID             int        `json:"id"`
	URL            string     `json:"url"`
	Interval       int        `json:"interval,omitempty"`
	Schedule       string     `json:"schedule,omitempty"`
	Overwrite      bool       `json:"overwrite"`
	LastRun        *time.Time `json:"last_run,omitempty"`
	Enabled        bool       `json:"enabled"`
	NameTemplate   string     `json:"name_template,omitempty"`
	SubdirTemplate string     `json:"subdir_template,omitempty"`
	HookTemplate   string     `json:"hook_template,omitempty"`
	EmitTemplate   string     `json:"emit_template,omitempty"`
}

func CreateJob(db *sql.DB, job Job) (int64, error) {
	result, err := db.Exec(`INSERT INTO jobs (url, interval, schedule, overwrite, last_run, enabled, name_template, subdir_template, hook_template, emit_template) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.URL, job.Interval, job.Schedule, job.Overwrite, job.LastRun, job.Enabled,
		job.NameTemplate, job.SubdirTemplate, job.HookTemplate, job.EmitTemplate)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetJob(db *sql.DB, id int) (*Job, error) {
	var job Job
	var lastRun sql.NullTime
	err := db.QueryRow(`SELECT id, url, interval, schedule, overwrite, last_run, enabled, name_template, subdir_template, hook_template, emit_template FROM jobs WHERE id = ?`,
		id).Scan(&job.ID, &job.URL, &job.Interval, &job.Schedule, &job.Overwrite, &lastRun, &job.Enabled,
		&job.NameTemplate, &job.SubdirTemplate, &job.HookTemplate, &job.EmitTemplate)
	if err != nil {
		return nil, err
	}
	if lastRun.Valid {
		job.LastRun = &lastRun.Time
	}
	return &job, nil
}

func ListJobs(db *sql.DB) ([]Job, error) {
	rows, err := db.Query(`SELECT id, url, interval, schedule, overwrite, last_run, enabled, name_template, subdir_template, hook_template, emit_template FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var lastRun sql.NullTime
		if err := rows.Scan(&job.ID, &job.URL, &job.Interval, &job.Schedule, &job.Overwrite, &lastRun, &job.Enabled,
			&job.NameTemplate, &job.SubdirTemplate, &job.HookTemplate, &job.EmitTemplate); err != nil {
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
	_, err := db.Exec(`UPDATE jobs SET url = ?, interval = ?, schedule = ?, overwrite = ?, last_run = ?, enabled = ?, name_template = ?, subdir_template = ?, hook_template = ?, emit_template = ? WHERE id = ?`,
		job.URL, job.Interval, job.Schedule, job.Overwrite, job.LastRun, job.Enabled,
		job.NameTemplate, job.SubdirTemplate, job.HookTemplate, job.EmitTemplate,
		job.ID)
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
