# yoinker

**Yoinker** is a lightweight Go service that schedules and downloads files over HTTP/HTTPS at configurable intervals.

## Features

- CRUD API for managing download jobs
- Concurrent downloads with configurable limits
- Persistent storage using SQLite
- Automatic retries on failure
- Graceful shutdown and context cancellation
- Health and metrics endpoints

## Requirements

- Go 1.24+

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/Takenobou/yoinker.git
   cd yoinker
   ```

2. Build and run locally:
   ```bash
   export DB_PATH=./data/yoinker.db       # path to database file or directory
   export DOWNLOAD_ROOT=./downloads       # root folder for downloaded files
   go run cmd/yoinker/main.go
   ```

3. Or build a static binary:
   ```bash
   CGO_ENABLED=0 GOOS=linux go build -a -o yoinker ./cmd/yoinker
   ```

4. Optionally use Docker:
   ```bash
   docker build -t takenobou/yoinker .
   docker run -d \
     -e DB_PATH=/data/yoinker.db \
     -e DOWNLOAD_ROOT=/data/downloads \
     -p 3000:3000 \
     -v yoinker_data:/data \
     takenobou/yoinker
   ```

5. Or use Docker Compose (included):
   ```bash
   docker-compose up -d
   ```

## Configuration

| Environment Variable         | Description                             | Default    |
|------------------------------|-----------------------------------------|------------|
| `DB_PATH` (required)         | SQLite file path or directory           |            |
| `PORT`                       | HTTP server port                        | `3000`     |
| `DOWNLOAD_ROOT`              | Directory for saving downloads          | `downloads`|
| `MAX_CONCURRENT_DOWNLOADS`   | Maximum parallel downloads              | `5`        |
| `LOG_LEVEL`                  | Logging level (`debug`,`info`,`error`)   | `info`     |

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
