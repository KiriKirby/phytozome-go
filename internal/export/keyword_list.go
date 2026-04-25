package export

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func WriteKeywordListText(path string, rows []model.KeywordResultRow) error {
	lines := buildKeywordListLines(rows)

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create keyword list text file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := bufio.NewWriterSize(file, 64*1024)
	for i, line := range lines {
		if i > 0 {
			if _, err := writer.WriteString("\n"); err != nil {
				return fmt.Errorf("write keyword list newline: %w", err)
			}
		}
		if _, err := writer.WriteString(line); err != nil {
			return fmt.Errorf("write keyword list line: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush keyword list text file: %w", err)
	}
	return nil
}

func buildKeywordListLines(rows []model.KeywordResultRow) []string {
	selected := append([]model.KeywordResultRow(nil), rows...)
	countByTerm := make(map[string]int, len(selected))
	hasProteinIdentification := false
	for _, row := range selected {
		countByTerm[strings.TrimSpace(row.SearchTerm)]++
		if strings.TrimSpace(row.ProteinIdentification) != "" {
			hasProteinIdentification = true
		}
	}

	if !hasProteinIdentification {
		lines := make([]string, 0, len(selected))
		for _, row := range selected {
			link := strings.TrimSpace(row.GeneReportURL)
			if link == "" {
				link = "~"
			}
			lines = append(lines, link)
		}
		return lines
	}

	lines := make([]string, 0, len(selected)*2+1)
	for _, row := range selected {
		lines = append(lines, keywordListLabel(row, countByTerm))
	}
	lines = append(lines, "~~")
	for _, row := range selected {
		link := strings.TrimSpace(row.GeneReportURL)
		if link == "" {
			link = "~"
		}
		lines = append(lines, link)
	}
	return lines
}

func keywordListLabel(row model.KeywordResultRow, countByTerm map[string]int) string {
	label := strings.TrimSpace(row.ProteinIdentification)
	if label == "" {
		label = "~"
	}
	term := strings.TrimSpace(row.SearchTerm)
	if countByTerm[term] > 1 && term != "" {
		label += " (" + term + ")"
	}
	return label
}
