package lemna

import (
	"os"

	"github.com/KiriKirby/phytozome-go/internal/cachex"
)

var lemnaCache = cachex.MustOpen("lemna")

func readCachedJSON[T any](group string, key string) (T, bool) {
	var value T
	if !lemnaCache.ReadJSON(group+":"+key, &value) {
		return value, false
	}
	return value, true
}

func writeCachedJSON(group string, key string, value any) {
	lemnaCache.WriteJSON(group+":"+key, value)
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
