package workflow

import (
	"context"
	"fmt"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
)

const lemnaDetectBlastCapabilityWorker = "workflow.lemna.detect_blast_capability"

type lemnaDetectBlastCapabilityInput struct {
	Species model.SpeciesCandidate `json:"species"`
}

type lemnaDetectBlastCapabilityOutput struct {
	Capability lemna.BlastCapability `json:"capability"`
}

func init() {
	phygoboost.Register(lemnaDetectBlastCapabilityWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input lemnaDetectBlastCapabilityInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode lemna BLAST capability input: %w", err)
		}
		client := lemna.NewClient(phygoboost.HTTPClient())
		if _, err := client.FetchSpeciesCandidates(ctx); err != nil {
			return nil, err
		}
		capability, err := client.DetectBlastCapabilities(ctx, input.Species)
		if err != nil {
			return nil, err
		}
		return json.Marshal(lemnaDetectBlastCapabilityOutput{Capability: capability})
	})
}

func detectLemnaBlastCapabilitiesProcess(ctx context.Context, selected model.SpeciesCandidate) (lemna.BlastCapability, bool, error) {
	if phygoboost.InWorker() {
		return lemna.BlastCapability{}, false, nil
	}
	var output lemnaDetectBlastCapabilityOutput
	err := phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Network:     map[string]int{"www.lemna.org": 1},
		Description: "detect lemna blast capability",
	}, lemnaDetectBlastCapabilityWorker, lemnaDetectBlastCapabilityInput{
		Species: selected,
	}, &output)
	return output.Capability, true, err
}


