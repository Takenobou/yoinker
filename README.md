# yoinker

**Yoinker** is a lightweight Go service that schedules and downloads files over HTTP/HTTPS at configurable intervals.

## Litmus Test

Is Yoinker right for you? Ask yourself:

- Do you need to fetch remote files on a schedule (interval or cron)?
- Would templated filenames or subdirectories save you manual work?
- Do you require duplicate suppression to avoid re‑processing identical data?
- Does your workflow benefit from post‑download hooks or pub/sub events?
- Are you looking for built‑in Prometheus metrics for observability?
- Do you host files behind a CDN or API that supports ETag/Last‑Modified caching?
- Would automatic unpacking (gzip, zip) and charset normalization streamline ingestion?
- Do you prefer a simple CLI for dev/test workflows alongside an HTTP API?

## Features

- CRUD API for managing download jobs
- Concurrent downloads with configurable limits
- Persistent storage using SQLite
- Automatic retries on failure
- Graceful shutdown and context cancellation
- Health and metrics endpoints
- Prometheus metrics endpoint (`/metrics`)
- Filename & subdirectory templating using Go `text/template`
- Duplicate download suppression (MD5-based)
- Post-download hooks and event emitters (shell commands or JSON)
- Cron and interval scheduling (`interval` or `schedule` via cron syntax)
- Templated URLs (e.g. `https://.../{{ .Now.Format "20060102" }}.csv`)
- Content-Disposition filename extraction
- Automatic unpacking of gzip/zip archives and charset transcoding to UTF-8
- HTTP caching (ETag/Last-Modified) and resumable downloads (HTTP Range)
- CLI tools: `yoinker add|ls|rm|stats|apply`

## Requirements

- Go 1.24+

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/Takenobou/yoinker.git
   cd yoinker
   ```

2. Build the binary and run in server mode:
   ```bash
   # Build the server (and CLI)
   export DB_PATH=./data/yoinker.db
   export DOWNLOAD_ROOT=./downloads
   # before running, ensure DB_PATH is explicitly set (no default fallback)
   go build -o yoinker cmd/yoinker/main.go
   # Start the HTTP server
   ./yoinker
   ```

3. CLI usage examples:
   ```bash
   # Add a job via CLI
   yoinker add --url=https://example.com/data.csv --interval=300
   # List jobs
   yoinker ls
   # View download stats
   yoinker stats
   # Remove a job
   yoinker rm <jobID>
   # Apply jobs from a YAML file
   yoinker apply jobs.yaml
   ```

4. Or build a static binary:
   ```bash
   CGO_ENABLED=0 GOOS=linux go build -a -o yoinker ./cmd/yoinker
   ```

5. Optionally use Docker:
   ```bash
   docker build -t takenobou/yoinker .
   docker run -d \
     -e DB_PATH=/data/yoinker.db \
     -e DOWNLOAD_ROOT=/data/downloads \
     -p 3000:3000 \
     -v yoinker_data:/data \
     takenobou/yoinker
   ```

6. Or use Docker Compose (included):
   ```bash
   docker-compose up -d
   ```

## Configuration

| Environment Variable         | Description                             | Default                                    |
|------------------------------|-----------------------------------------|--------------------------------------------|
| `DB_PATH`                    | SQLite file path or directory           | _required_                                 |
| `PORT`                       | HTTP server port                        | `3000`                                     |
| `DOWNLOAD_ROOT`              | Directory for saving downloads          | `downloads`                                |
| `MAX_CONCURRENT_DOWNLOADS`   | Maximum parallel downloads              | `5`                                        |
| `LOG_LEVEL`                  | Logging level (`debug`,`info`,`error`)  | `info`                                     |

> If `DB_PATH` points to a directory, `yoinker.db` will be appended automatically.

## API Endpoints

### Jobs

- `GET /jobs` 
  List all jobs

- `GET /jobs/{id}`
  Get details of a specific job

- `POST /jobs`
  Create a new job
  ```json
  { "url": "https://example.com/file.txt", "interval": 60, "overwrite": false }
  ```

- `PUT /jobs/{id}`
  Update an existing job
  ```json
  { "url": "https://example.com/file.txt", "interval": 120, "overwrite": true, "enabled": true }
  ```

- `DELETE /jobs/{id}`
  Remove a job

### Health

- `GET /health` 
  Returns `OK` if service is up

- `GET /health/details`
  Returns JSON with uptime, database status, goroutines, memory stats
