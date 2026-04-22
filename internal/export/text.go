package export

import (
	"fmt"
	"os"
	"strings"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
)

func WriteProteinSequencesText(path string, records []model.ProteinSequenceRecord) error {
	var builder strings.Builder
	for idx, record := range records {
		if idx > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(record.Header)
		builder.WriteString("\n")
		builder.WriteString(record.Sequence)
	}

	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		return fmt.Errorf("write text file: %w", err)
	}
	return nil
}
