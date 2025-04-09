package download

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

func DownloadFile(url, dest string, overwrite bool, logger *zap.Logger) (string, error) {
	logger.Info("Downloading file", zap.String("url", url))

	resp, err := http.Get(url)
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

	if fileExists(dest) && !overwrite {
		dest = timestampedFilename(dest)
		logger.Info("File exists, new file will be created", zap.String("dest", dest))
	}

	out, err := os.Create(dest)
	if err != nil {
		logger.Error("File creation error", zap.Error(err))
		return "", err
	}
	defer out.Close()

	hasher := md5.New()
	writer := io.MultiWriter(out, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		logger.Error("File write error", zap.Error(err))
		return "", err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	logger.Info("Download complete", zap.String("dest", dest), zap.String("hash", hash))
	return hash, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func timestampedFilename(original string) string {
	dir := filepath.Dir(original)
	ext := path.Ext(original)
	base := strings.TrimSuffix(path.Base(original), ext)
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext))
}
