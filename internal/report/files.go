package report

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func InspectGeneratedFile(path string, fileType string, role string, hashTime time.Time) (GeneratedFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return GeneratedFile{}, fmt.Errorf("stat generated file: %w", err)
	}
	sha256Value, sha1Value, md5Value, err := hashFile(path)
	if err != nil {
		return GeneratedFile{}, err
	}
	if hashTime.IsZero() {
		hashTime = time.Now()
	}
	return GeneratedFile{
		Name:         filepath.Base(path),
		Type:         fileType,
		Role:         role,
		Path:         path,
		SizeBytes:    info.Size(),
		ModifiedAt:   info.ModTime(),
		Permissions:  info.Mode().String(),
		Owner:        "not available in this run",
		SHA256:       sha256Value,
		SHA1:         sha1Value,
		MD5:          md5Value,
		HashCaptured: hashTime,
	}, nil
}

func PlannedReportFile(path string, generatedAt time.Time) GeneratedFile {
	return GeneratedFile{
		Name:         filepath.Base(path),
		Type:         "report PDF",
		Role:         "Data Analysis Report for the current export action",
		Path:         path,
		SizeBytes:    -1,
		CreatedAt:    generatedAt,
		ModifiedAt:   generatedAt,
		AccessedAt:   generatedAt,
		Permissions:  "not available before final PDF is written",
		Owner:        "not available in this run",
		SHA256:       "not embedded in this PDF; a PDF cannot reliably contain its own final hash without changing its bytes",
		SHA1:         "not embedded in this PDF",
		MD5:          "not embedded in this PDF",
		HashCaptured: generatedAt,
	}
}

func hashFile(path string) (string, string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", "", fmt.Errorf("open generated file for hash: %w", err)
	}
	defer file.Close()

	sha256Hash := sha256.New()
	sha1Hash := sha1.New()
	md5Hash := md5.New()
	if _, err := io.Copy(io.MultiWriter(sha256Hash, sha1Hash, md5Hash), file); err != nil {
		return "", "", "", fmt.Errorf("hash generated file: %w", err)
	}
	return hex.EncodeToString(sha256Hash.Sum(nil)),
		hex.EncodeToString(sha1Hash.Sum(nil)),
		hex.EncodeToString(md5Hash.Sum(nil)),
		nil
}
