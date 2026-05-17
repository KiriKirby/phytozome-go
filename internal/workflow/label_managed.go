package workflow

import (
	"context"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func autoIdentifyBlastLabelManaged(ctx context.Context, selected model.SpeciesCandidate, item blastQueryItem) (blastAutoLabelResult, bool, error) {
	wizard, err := managedWizardForDatabase("phytozome")
	if err != nil {
		return blastAutoLabelResult{}, true, err
	}
	var result blastAutoLabelResult
	err = phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "phytozome-next.jgi.doe.gov",
		Description: "auto identify blast label",
	}, func(runCtx context.Context) error {
		result = wizard.autoIdentifyBlastLabelResult(runCtx, wizard.source, selected, item)
		return nil
	})
	return result, true, err
}
