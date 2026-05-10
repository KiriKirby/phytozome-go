package report

import (
	"context"
	"fmt"
	"time"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
)

const InspectGeneratedFileWorker = "report.inspect_generated_file"
const RenderKeywordPDFWorker = "report.render_keyword_pdf"
const RenderBlastPDFWorker = "report.render_blast_pdf"

type InspectGeneratedFileInput struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Role string `json:"role"`
}

type renderPDFInput struct {
	Path string     `json:"path"`
	Data ReportData `json:"data"`
}

func init() {
	phygoboost.RegisterIndexedJSON[InspectGeneratedFileInput, GeneratedFile](InspectGeneratedFileWorker, func(ctx context.Context, index int, input InspectGeneratedFileInput) (GeneratedFile, error) {
		_ = index
		return phygoboost.RunDiskValue(ctx, func(ctx context.Context) (GeneratedFile, error) {
			return InspectGeneratedFile(input.Path, input.Type, input.Role, time.Now())
		})
	})
	phygoboost.Register(RenderKeywordPDFWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input renderPDFInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode keyword PDF worker input: %w", err)
		}
		return nil, phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return RenderKeywordPDF(input.Path, input.Data)
		})
	})
	phygoboost.Register(RenderBlastPDFWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input renderPDFInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode BLAST PDF worker input: %w", err)
		}
		return nil, phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return RenderBlastPDF(input.Path, input.Data)
		})
	})
}

func RenderKeywordPDFProcess(ctx context.Context, path string, data ReportData) error {
	if phygoboost.InWorker() {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return RenderKeywordPDF(path, data)
		})
	}
	return phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "render keyword pdf"}, RenderKeywordPDFWorker, renderPDFInput{Path: path, Data: data}, (*struct{})(nil))
}

func RenderBlastPDFProcess(ctx context.Context, path string, data ReportData) error {
	if phygoboost.InWorker() {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return RenderBlastPDF(path, data)
		})
	}
	return phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "render blast pdf"}, RenderBlastPDFWorker, renderPDFInput{Path: path, Data: data}, (*struct{})(nil))
}

