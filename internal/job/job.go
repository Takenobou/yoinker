package job

import (
	"database/sql"
	"os"
	"time"
)

type Job struct {
	ID             int        `json:"id"`
	URL            string     `json:"url"`
	Interval       int        `json:"interval,omitempty"`
	Schedule       string     `json:"schedule,omitempty"`
	Overwrite      bool       `json:"overwrite"`
	LastRun        *time.Time `json:"last_run,omitempty"`
	LastETag       string     `json:"last_etag,omitempty"`
	LastModified   string     `json:"last_modified,omitempty"`
	Enabled        bool       `json:"enabled"`
	NameTemplate   string     `json:"name_template,omitempty"`
	SubdirTemplate string     `json:"subdir_template,omitempty"`
	HookTemplate   string     `json:"hook_template,omitempty"`
	EmitTemplate   string     `json:"emit_template,omitempty"`
}

func CreateJob(db *sql.DB, job Job) (int64, error) {
	result, err := db.Exec(`INSERT INTO jobs (url, interval, schedule, overwrite, last_run, last_etag, last_modified, enabled, name_template, subdir_template, hook_template, emit_template) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.URL, job.Interval, job.Schedule, job.Overwrite, job.LastRun, job.LastETag, job.LastModified, job.Enabled,
		job.NameTemplate, job.SubdirTemplate, job.HookTemplate, job.EmitTemplate)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetJob(db *sql.DB, id int) (*Job, error) {
	var job Job
	var lastRun sql.NullTime
	err := db.QueryRow(`SELECT id, url, interval, schedule, overwrite, last_run, last_etag, last_modified, enabled, name_template, subdir_template, hook_template, emit_template FROM jobs WHERE id = ?`,
		id).Scan(&job.ID, &job.URL, &job.Interval, &job.Schedule, &job.Overwrite, &lastRun, &job.LastETag, &job.LastModified, &job.Enabled,
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
	rows, err := db.Query(`SELECT id, url, interval, schedule, overwrite, last_run, last_etag, last_modified, enabled, name_template, subdir_template, hook_template, emit_template FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var lastRun sql.NullTime
		if err := rows.Scan(&job.ID, &job.URL, &job.Interval, &job.Schedule, &job.Overwrite, &lastRun, &job.LastETag, &job.LastModified, &job.Enabled,
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
	_, err := db.Exec(`UPDATE jobs SET url = ?, interval = ?, schedule = ?, overwrite = ?, last_run = ?, last_etag = ?, last_modified = ?, enabled = ?, name_template = ?, subdir_template = ?, hook_template = ?, emit_template = ? WHERE id = ?`,
		job.URL, job.Interval, job.Schedule, job.Overwrite, job.LastRun, job.LastETag, job.LastModified, job.Enabled,
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

// PruneDownloads deletes downloads older than the given time and returns number of records removed.
func PruneDownloads(db *sql.DB, cutoff time.Time) (int64, error) {
	// Collect file paths to remove
	rows, err := db.Query(`SELECT file_path FROM downloads WHERE downloaded_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths = append(paths, p)
		}
	}

	// Delete DB rows
	res, err := db.Exec(`DELETE FROM downloads WHERE downloaded_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	// Remove files from disk
	for _, p := range paths {
		os.Remove(p)
	}
	return count, nil
}
