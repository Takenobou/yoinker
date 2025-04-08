# Build stage
FROM golang:1.24-alpine AS builder

# Install git; required for go mod download if necessary
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod and sum files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -o yoinker ./cmd/yoinker

# Final stage: use distroless base image for a minimal container
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from the builder
COPY --from=builder /app/yoinker /yoinker

# Expose the application's port (default 3000)
EXPOSE 3000

# Run the binary.
ENTRYPOINT ["/yoinker"]