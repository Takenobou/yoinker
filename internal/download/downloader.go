package download

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// DownloadFile downloads a file from the given URL. If overwrite is false and the destination file exists, a new file is created with a timestamp appended.
func DownloadFile(url, dest string, overwrite bool, logger *zap.Logger) error {
	logger.Info("Downloading file", zap.String("url", url))

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("HTTP GET error", zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad status: %s", resp.Status)
		logger.Error("HTTP GET failed", zap.Error(err))
		return err
	}

	if _, err = os.Stat(dest); err == nil && !overwrite {
		dest = fmt.Sprintf("%s_%d", dest, time.Now().Unix())
		logger.Info("File exists, new file will be created", zap.String("dest", dest))
	}

	out, err := os.Create(dest)
	if err != nil {
		logger.Error("File creation error", zap.Error(err))
		return err
	}
	defer out.Close()

	hasher := md5.New()
	writer := io.MultiWriter(out, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		logger.Error("File write error", zap.Error(err))
		return err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	logger.Info("Download complete", zap.String("dest", dest), zap.String("hash", hash))

	return nil
}
