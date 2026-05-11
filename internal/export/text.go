package export

import (
	"bufio"
	"fmt"
	"os"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func WriteProteinSequencesText(path string, records []model.ProteinSequenceRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create text file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := bufio.NewWriterSize(file, 64*1024)
	for idx, record := range records {
		if idx > 0 {
			if _, err := writer.WriteString("\n\n"); err != nil {
				return fmt.Errorf("write text separator: %w", err)
			}
		}
		if _, err := writer.WriteString(record.Header); err != nil {
			return fmt.Errorf("write text header: %w", err)
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return fmt.Errorf("write text newline: %w", err)
		}
		if _, err := writer.WriteString(record.Sequence); err != nil {
			return fmt.Errorf("write text sequence: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush text file: %w", err)
	}
	return nil
}
