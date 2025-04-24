package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Takenobou/yoinker/internal/util"
	"go.uber.org/zap"
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

	// Stream download directly to file
	if _, err := io.Copy(out, resp.Body); err != nil {
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
