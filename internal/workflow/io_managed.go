package workflow

import (
	"context"
	"os"

	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func readTextFileManaged(ctx context.Context, path string) (string, bool, error) {
	var data string
	err := phygoboost.RunDisk(ctx, func(context.Context) error {
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		data = string(bytes)
		return nil
	})
	return data, true, err
}

func installBlastPlusManaged(ctx context.Context) (string, bool, error) {
	var binDir string
	err := phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecManaged, Description: "install managed blast+"}, func(runCtx context.Context) error {
		var err error
		binDir, err = blastplus.InstallManaged(runCtx, phygoboost.HTTPClient())
		return err
	})
	return binDir, true, err
}
