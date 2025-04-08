package util

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
)

// ComputeFileMD5 computes the MD5 hash of the file at the specified path.
func ComputeFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// FilesEqual compares the MD5 hash of two files to determine if they are identical.
func FilesEqual(filePath1, filePath2 string) (bool, error) {
	hash1, err := ComputeFileMD5(filePath1)
	if err != nil {
		return false, err
	}

	hash2, err := ComputeFileMD5(filePath2)
	if err != nil {
		return false, err
	}

	return hash1 == hash2, nil
}
