// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package appfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var clearRunCacheOnce sync.Once

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

func CacheDir(parts ...string) (string, error) {
	base, err := CacheRoot()
	if err != nil {
		return "", err
	}
	dir := base
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dir = filepath.Join(dir, part)
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

func ResetRunCache() error {
	roots, err := cacheRootPaths()
	if err != nil {
		return err
	}
	for _, base := range roots {
		if err := os.RemoveAll(base); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reset cache directory %s: %w", base, err)
		}
	}
	return nil
}

func ResetRunCacheOnce() error {
	var resetErr error
	clearRunCacheOnce.Do(func() {
		resetErr = ResetRunCache()
	})
	return resetErr
}

func cacheRootPaths() ([]string, error) {
	seen := make(map[string]struct{}, 2)
	roots := make([]string, 0, 2)
	add := func(base string) {
		base = strings.TrimSpace(base)
		if base == "" {
			return
		}
		base = filepath.Clean(base)
		key := strings.ToLower(base)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		roots = append(roots, base)
	}

	appDir, err := ApplicationDir()
	if err != nil {
		return nil, err
	}
	add(filepath.Join(appDir, ".cache"))

	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, ".cache"))
	}
	return roots, nil
}

func RemoveCacheSubtree(parts ...string) error {
	root, err := CacheRoot()
	if err != nil {
		return err
	}
	target := root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		target = filepath.Join(target, part)
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cache subtree %s: %w", target, err)
	}
	return nil
}
