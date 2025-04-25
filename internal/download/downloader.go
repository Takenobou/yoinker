package download

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Takenobou/yoinker/internal/util"
	"go.uber.org/zap"
	"golang.org/x/net/html/charset"
)

// ErrNotModified indicates a 304 Not Modified response
var ErrNotModified = errors.New("not modified")

// DownloadFile downloads a file from the given URL and writes it to dest.
// This is a wrapper around DownloadFileWithContext using a background context for backward compatibility.
func DownloadFile(url, dest string, overwrite bool, logger *zap.Logger) (string, error) {
	return DownloadFileWithContext(context.Background(), url, dest, overwrite, logger)
}

// DownloadFileWithContext downloads a file from the given URL and writes it to dest.
// It respects context cancellation for graceful shutdown. Uses extended version under the hood.
func DownloadFileWithContext(ctx context.Context, url, dest string, overwrite bool, logger *zap.Logger) (string, error) {
	hash, _, _, err := DownloadFileExtended(ctx, url, dest, overwrite, "", "", logger)
	return hash, err
}

// DownloadFileExtended downloads a file supporting ETag/If-Modified-Since short-circuit and HTTP Range resumes
func DownloadFileExtended(ctx context.Context, url, dest string, overwrite bool, lastETag, lastModified string, logger *zap.Logger) (string, string, string, error) {
	logger.Info("Downloading file", zap.String("url", url))

	// Check for resumable download
	tempDest := dest + ".download"
	var out *os.File
	var offset int64
	if fi, err := os.Stat(tempDest); err == nil {
		// Resume
		offset = fi.Size()
		out, err = os.OpenFile(tempDest, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return "", "", "", err
		}
		logger.Info("Resuming download", zap.Int64("offset", offset))
	} else {
		out, err = os.Create(tempDest)
		if err != nil {
			logger.Error("File creation error", zap.Error(err))
			return "", "", "", err
		}
	}
	defer func() { out.Close() }()

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Error("Failed to create request", zap.Error(err))
		return "", "", "", err
	}

	// Add headers for ETag/Last-Modified
	if lastETag != "" {
		req.Header.Set("If-None-Match", lastETag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	// Range header for resumable
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("HTTP GET error", zap.Error(err))
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		logger.Info("Not modified (304), skipping download")
		return "", resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), ErrNotModified
	}
	if !(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent) {
		err = fmt.Errorf("bad status: %s", resp.Status)
		logger.Error("HTTP GET failed", zap.Error(err))
		return "", "", "", err
	}

	// Write body to file
	if _, err := io.Copy(out, resp.Body); err != nil {
		logger.Error("Error writing to file", zap.Error(err))
		return "", "", "", err
	}
	// Finalize temp file
	out.Close()
	if err := os.Rename(tempDest, dest); err != nil {
		logger.Error("Error renaming file", zap.Error(err))
		return "", "", "", err
	}

	// Determine filename override from Content-Disposition
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if filename, ok := params["filename"]; ok {
				dir := filepath.Dir(dest)
				dest = filepath.Join(dir, filename)
				logger.Info("Applying Content-Disposition filename", zap.String("filename", filename))
			}
		}
	}

	if !overwrite {
		dest = TimestampedFilename(dest)
		logger.Info("Overwrite disabled, appending timestamp", zap.String("dest", dest))
	} else if fileExists(dest) {
		logger.Info("Overwrite enabled, existing file will be overwritten", zap.String("dest", dest))
	}

	// If gzip unpack removed .gz suffix, adjust dest
	if strings.HasSuffix(strings.ToLower(dest), ".gz") {
		unpacked := strings.TrimSuffix(dest, ".gz")
		f, err := os.Open(dest)
		if err == nil {
			if gz, err := gzip.NewReader(f); err == nil {
				data, _ := io.ReadAll(gz)
				gz.Close()
				os.WriteFile(unpacked, data, 0644)
				f.Close()
				os.Remove(dest)
				dest = unpacked
				logger.Info("Gzip unpacked file", zap.String("path", dest))
			}
		}
	}

	// Auto-unpack zip (assumes single file archive)
	if strings.HasSuffix(strings.ToLower(dest), ".zip") {
		if zr, err := zip.OpenReader(dest); err == nil {
			defer zr.Close()
			destDir := filepath.Dir(dest)
			// Extract first file
			for _, f := range zr.File {
				if f.FileInfo().IsDir() {
					os.MkdirAll(filepath.Join(destDir, f.Name), f.Mode())
					continue
				}
				rc, err := f.Open()
				if err != nil {
					continue
				}
				outPath := filepath.Join(destDir, f.Name)
				os.MkdirAll(filepath.Dir(outPath), 0755)
				outFile, err := os.Create(outPath)
				if err == nil {
					io.Copy(outFile, rc)
					outFile.Close()
				}
				rc.Close()
				// Remove zip and set dest to extracted file
				os.Remove(dest)
				dest = outPath
				break
			}
		}
	}

	// Transcode charset to UTF-8 if needed
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		lower := strings.ToLower(ct)
		if idx := strings.Index(lower, "charset="); idx != -1 && !strings.Contains(lower, "utf-8") {
			cs := lower[idx+8:]
			raw, err := os.ReadFile(dest)
			if err == nil {
				if r, err := charset.NewReaderLabel(cs, bytes.NewReader(raw)); err == nil {
					data, _ := io.ReadAll(r)
					os.WriteFile(dest, data, 0644)
					logger.Info("Transcoded file to UTF-8", zap.String("path", dest))
				}
			}
		}
	}

	// Compute MD5 using helper
	hash, err := util.ComputeFileMD5(dest)
	if err != nil {
		logger.Error("Failed to compute MD5", zap.Error(err))
		return "", "", "", err
	}
	logger.Info("Download complete", zap.String("dest", dest), zap.String("hash", hash))
	return hash, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TimestampedFilename(original string) string {
	dir := filepath.Dir(original)
	ext := path.Ext(original)
	base := strings.TrimSuffix(path.Base(original), ext)
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext))
}
