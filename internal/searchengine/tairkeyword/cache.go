package tairkeyword

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

func cacheFilePath(group string, key string, ext string) (string, error) {
	dir, err := appfs.CacheDir("tair", "searchengine", group)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+ext), nil
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
	_ = appfs.WriteFileAtomic(path, data, 0o644)
}
