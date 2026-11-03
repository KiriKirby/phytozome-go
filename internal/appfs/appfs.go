// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package appfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var atomicWriteLocks sync.Map

var (
	ErrFolderSelectionCancelled = errors.New("folder selection cancelled")
	ErrFileSelectionCancelled   = errors.New("file selection cancelled")
)

func ApplicationDir() (string, error) {
	executablePath, err := os.Executable()
	if err == nil {
		executableDir := filepath.Dir(executablePath)
		if !strings.Contains(strings.ToLower(executableDir), strings.ToLower(os.TempDir())) {
			return executableDir, nil
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve application directory: %w", err)
	}
	return workingDir, nil
}

func OutputDir() (string, error) {
	appDir, err := ApplicationDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(appDir, "output")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensure output directory: %w", err)
	}
	return dir, nil
}

func SelectFolder(title string, defaultDir string) (string, error) {
	defaultDir = strings.TrimSpace(defaultDir)
	if defaultDir == "" {
		var err error
		defaultDir, err = OutputDir()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure default output directory: %w", err)
	}
	if strings.TrimSpace(os.Getenv("PHYTOZOME_GO_DISABLE_FOLDER_PICKER")) != "" {
		return filepath.Abs(defaultDir)
	}
	selected, err := selectFolderSystem(title, defaultDir)
	if err != nil {
		if errors.Is(err, ErrFolderSelectionCancelled) {
			return "", err
		}
		return filepath.Abs(defaultDir)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return filepath.Abs(defaultDir)
	}
	if err := os.MkdirAll(selected, 0o755); err != nil {
		return "", fmt.Errorf("ensure selected output directory: %w", err)
	}
	return filepath.Abs(selected)
}

func SelectFile(title string, defaultDir string) (string, error) {
	defaultDir = strings.TrimSpace(defaultDir)
	if defaultDir == "" {
		var err error
		defaultDir, err = ApplicationDir()
		if err != nil {
			return "", err
		}
	}
	selected, err := selectFileSystem(title, defaultDir)
	if err != nil {
		if errors.Is(err, ErrFileSelectionCancelled) {
			return "", err
		}
		return "", err
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", ErrFileSelectionCancelled
	}
	return filepath.Abs(selected)
}

func CacheDir(parts ...string) (string, error) {
	base, err := CacheRoot()
	if err != nil {
		return "", err
	}
	dir, err := cachePath(base, parts...)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensure cache directory: %w", err)
	}
	return dir, nil
}

func CacheRoot() (string, error) {
	appDir, err := ApplicationDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(appDir, ".cache")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("ensure cache directory: %w", err)
	}
	markHiddenIfSupported(base)
	return base, nil
}

func RemoveCacheSubtree(parts ...string) error {
	root, err := CacheRoot()
	if err != nil {
		return err
	}
	target, err := cachePath(root, parts...)
	if err != nil {
		return err
	}
	if samePath(target, root) {
		return fmt.Errorf("refusing to remove entire cache root from the main program")
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cache subtree %s: %w", target, err)
	}
	return nil
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("empty atomic write path")
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("resolve atomic write path: %w", err)
	}
	lockValue, _ := atomicWriteLocks.LoadOrStore(absPath, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	path = absPath
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure atomic write directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temporary file permissions for %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := replaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s atomically: %w", path, err)
	}
	cleanup = false
	return nil
}

func cachePath(root string, parts ...string) (string, error) {
	root = filepath.Clean(root)
	target := root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned := filepath.Clean(part)
		if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe cache path component %q", part)
		}
		target = filepath.Join(target, cleaned)
	}
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve cache path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("cache path escapes cache root: %s", target)
	}
	return target, nil
}

func samePath(a string, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
