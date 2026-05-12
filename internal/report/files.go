// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

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
	"strings"
	"sync"
	"time"
)

type GeneratedFileInspector struct {
	mu    sync.RWMutex
	cache map[string]GeneratedFile
}

func NewGeneratedFileInspector() *GeneratedFileInspector {
	return &GeneratedFileInspector{cache: map[string]GeneratedFile{}}
}

func InspectGeneratedFile(path string, fileType string, role string, hashTime time.Time) (GeneratedFile, error) {
	return inspectGeneratedFileCached(nil, path, fileType, role, hashTime)
}

func (i *GeneratedFileInspector) Inspect(path string, fileType string, role string, hashTime time.Time) (GeneratedFile, error) {
	return inspectGeneratedFileCached(i, path, fileType, role, hashTime)
}

func inspectGeneratedFileCached(inspector *GeneratedFileInspector, path string, fileType string, role string, hashTime time.Time) (GeneratedFile, error) {
	cacheKey := normalizedGeneratedFilePath(path)
	if inspector != nil {
		if cached, ok := inspector.cached(cacheKey); ok {
			cached.Type = fileType
			cached.Role = role
			if !hashTime.IsZero() {
				cached.HashCaptured = hashTime
			}
			return cached, nil
		}
	}
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
	file := GeneratedFile{
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
	}
	if inspector != nil {
		inspector.store(cacheKey, file)
	}
	return file, nil
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

func normalizedGeneratedFilePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func (i *GeneratedFileInspector) cached(key string) (GeneratedFile, bool) {
	if i == nil || key == "" {
		return GeneratedFile{}, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	file, ok := i.cache[key]
	return file, ok
}

func (i *GeneratedFileInspector) store(key string, file GeneratedFile) {
	if i == nil || key == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cache[key] = file
}
