package lemna

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

func cacheFilePath(group string, key string, ext string) (string, error) {
	dir, err := appfs.CacheDir("lemna", group)
	if err != nil {
		return "", err
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

type proteinTranscriptDisk struct {
	ProtToTrans map[string]string `json:"prot_to_trans"`
	TransToGene map[string]string `json:"trans_to_gene"`
}

type fastaIndexDiskEntry struct {
	Defline string `json:"defline"`
	Length  int    `json:"length"`
}
