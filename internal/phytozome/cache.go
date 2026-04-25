package phytozome

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

func cacheFilePath(group string, key string, ext string) (string, error) {
	dir, err := appfs.CacheDir("phytozome", group)
	if err != nil {
		return "", fmt.Errorf("ensure phytozome cache dir: %w", err)
	}
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(dir, name+ext), nil
}

func readCachedJSON[T any](group string, key string) (T, bool) {
	var zero T
	path, err := cacheFilePath(group, key, ".json")
	if err != nil {
		return zero, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, false
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return zero, false
	}
	return value, true
}

func writeCachedJSON(group string, key string, value any) {
	path, err := cacheFilePath(group, key, ".json")
	if err != nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = writeAtomically(path, data)
}

func readCachedText(group string, key string) (string, bool) {
	path, err := cacheFilePath(group, key, ".txt")
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func writeCachedText(group string, key string, value string) {
	path, err := cacheFilePath(group, key, ".txt")
	if err != nil {
		return
	}
	_ = writeAtomically(path, []byte(value))
}

func writeAtomically(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
