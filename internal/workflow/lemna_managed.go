package workflow

import (
	"context"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func detectLemnaBlastCapabilitiesManaged(ctx context.Context, selected model.SpeciesCandidate) (lemna.BlastCapability, bool, error) {
	client := lemna.NewClient(phygoboost.HTTPClient())
	var capability lemna.BlastCapability
	err := phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Network:     map[string]int{"www.lemna.org": 1},
		Description: "detect lemna blast capability",
	}, func(runCtx context.Context) error {
		if _, err := client.FetchSpeciesCandidates(runCtx); err != nil {
			return err
		}
		var err error
		capability, err = client.DetectBlastCapabilities(runCtx, selected)
		return err
	})
	return capability, true, err
}
