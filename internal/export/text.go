// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

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
