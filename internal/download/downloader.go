package download

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
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

// DownloadFile downloads a file from the given URL and writes it to dest.
// This is a wrapper around DownloadFileWithContext using a background context for backward compatibility.
func DownloadFile(url, dest string, overwrite bool, logger *zap.Logger) (string, error) {
	return DownloadFileWithContext(context.Background(), url, dest, overwrite, logger)
}

// DownloadFileWithContext downloads a file from the given URL and writes it to dest.
// It respects context cancellation for graceful shutdown.
func DownloadFileWithContext(ctx context.Context, url, dest string, overwrite bool, logger *zap.Logger) (string, error) {
	logger.Info("Downloading file", zap.String("url", url))

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Error("Failed to create request", zap.Error(err))
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("HTTP GET error", zap.Error(err))
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad status: %s", resp.Status)
		logger.Error("HTTP GET failed", zap.Error(err))
		return "", err
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

	// Create a temporary file first, then rename on success
	tempDest := dest + ".download"
	out, err := os.Create(tempDest)
	if err != nil {
		logger.Error("File creation error", zap.Error(err))
		return "", err
	}
	defer func() {
		out.Close()
		// Clean up temp file on failure
		if err != nil {
			os.Remove(tempDest)
		}
	}()

	// Determine if gzip or zip unpack
	reader := resp.Body
	// auto-unpack gzip
	if strings.HasSuffix(strings.ToLower(url), ".gz") || strings.HasSuffix(strings.ToLower(dest), ".gz") {
		if gz, err := gzip.NewReader(resp.Body); err == nil {
			reader = gz
			defer gz.Close()
		}
	}

	// Stream download directly to file
	if _, err := io.Copy(out, reader); err != nil {
		logger.Error("Error writing to file", zap.Error(err))
		return "", err
	}

	// Close the file before renaming
	if err = out.Close(); err != nil {
		logger.Error("Error closing file", zap.Error(err))
		return "", err
	}

	// Rename temporary file to final destination
	if err = os.Rename(tempDest, dest); err != nil {
		logger.Error("Error renaming file", zap.Error(err))
		return "", err
	}

	// If gzip unpack removed .gz suffix, adjust dest
	if strings.HasSuffix(strings.ToLower(dest), ".gz") {
		unpacked := strings.TrimSuffix(dest, ".gz")
		f, err := os.Open(dest)
		if err == nil {
			if gz, err := gzip.NewReader(f); err == nil {
				data, _ := ioutil.ReadAll(gz)
				gz.Close()
				ioutil.WriteFile(unpacked, data, 0644)
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
			raw, err := ioutil.ReadFile(dest)
			if err == nil {
				if r, err := charset.NewReaderLabel(cs, bytes.NewReader(raw)); err == nil {
					data, _ := io.ReadAll(r)
					ioutil.WriteFile(dest, data, 0644)
					logger.Info("Transcoded file to UTF-8", zap.String("path", dest))
				}
			}
		}
	}

	// Compute MD5 using helper
	hash, err := util.ComputeFileMD5(dest)
	if err != nil {
		logger.Error("Failed to compute MD5", zap.Error(err))
		return "", err
	}
	logger.Info("Download complete", zap.String("dest", dest), zap.String("hash", hash))
	return hash, nil
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
