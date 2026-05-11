package appfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func CacheDir(parts ...string) (string, error) {
	appDir, err := ApplicationDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(appDir, ".cache")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("ensure cache directory: %w", err)
	}
	markHiddenIfSupported(base)

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
