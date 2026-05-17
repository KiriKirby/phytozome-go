package report

import (
	"context"
	"fmt"
	"time"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

type InspectGeneratedFileInput struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func InspectGeneratedFilesManaged(ctx context.Context, inputs []InspectGeneratedFileInput, outputs []GeneratedFile) error {
	if len(inputs) == 0 {
		return nil
	}
	if len(outputs) < len(inputs) {
		return fmt.Errorf("generated file output slice too small: have %d, need %d", len(outputs), len(inputs))
	}
	return phygoboost.ParallelForSpec(ctx, phygoboost.ParallelSpec{
		Level:       phygoboost.ExecManaged,
		Description: "inspect generated files",
	}, len(inputs), func(runCtx context.Context, index int) error {
		value, err := phygoboost.RunDiskValue(runCtx, func(context.Context) (GeneratedFile, error) {
			input := inputs[index]
			return InspectGeneratedFile(input.Path, input.Type, input.Role, time.Now())
		})
		if err != nil {
			return err
		}
		outputs[index] = value
		return nil
	})
}

func RenderKeywordPDFManaged(ctx context.Context, path string, data ReportData) error {
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		return RenderKeywordPDF(path, data)
	})
}

func RenderBlastPDFManaged(ctx context.Context, path string, data ReportData) error {
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		return RenderBlastPDF(path, data)
	})
}
