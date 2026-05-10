package workflow

import (
	"context"
	"fmt"
	"os"

	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
)

const (
	readTextFileWorker        = "workflow.io.read_text_file"
	installManagedBlastWorker = "workflow.blastplus.install_managed"
)

type readTextFileInput struct {
	Path string `json:"path"`
}

type readTextFileOutput struct {
	Data string `json:"data"`
}

type installManagedBlastOutput struct {
	BinDir string `json:"bin_dir"`
}

func init() {
	phygoboost.Register(readTextFileWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input readTextFileInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode text file input: %w", err)
		}
		data, err := phygoboost.RunDiskValue(ctx, func(context.Context) ([]byte, error) {
			return os.ReadFile(input.Path)
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(readTextFileOutput{Data: string(data)})
	})
	phygoboost.Register(installManagedBlastWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		binDir, err := blastplus.InstallManagedWithProgress(ctx, phygoboost.HTTPClient(), nil)
		if err != nil {
			return nil, err
		}
		return json.Marshal(installManagedBlastOutput{BinDir: binDir})
	})
}

func readTextFileProcess(ctx context.Context, path string) (string, bool, error) {
	if phygoboost.InWorker() {
		return "", false, nil
	}
	var output readTextFileOutput
	err := phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "read text file"}, readTextFileWorker, readTextFileInput{Path: path}, &output)
	return output.Data, true, err
}

func installManagedBlastProcess(ctx context.Context) (string, bool, error) {
	if phygoboost.InWorker() {
		return "", false, nil
	}
	var output installManagedBlastOutput
	err := phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "install managed blast+"}, installManagedBlastWorker, struct{}{}, &output)
	return output.BinDir, true, err
}


