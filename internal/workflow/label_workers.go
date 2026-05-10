package workflow

import (
	"context"
	"fmt"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
)

const autoIdentifyBlastLabelWorker = "workflow.blast.auto_identify_label"

type autoIdentifyBlastLabelInput struct {
	Selected model.SpeciesCandidate `json:"selected"`
	Item     blastQueryItem         `json:"item"`
}

type autoIdentifyBlastLabelOutput struct {
	Result blastAutoLabelResult `json:"result"`
}

func init() {
	phygoboost.Register(autoIdentifyBlastLabelWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input autoIdentifyBlastLabelInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode BLAST label input: %w", err)
		}
		wizard, err := workerWizardForDatabase("phytozome")
		if err != nil {
			return nil, err
		}
		result := wizard.autoIdentifyBlastLabelResult(ctx, wizard.source, input.Selected, input.Item)
		return json.Marshal(autoIdentifyBlastLabelOutput{Result: result})
	})
}

func autoIdentifyBlastLabelProcess(ctx context.Context, selected model.SpeciesCandidate, item blastQueryItem) (blastAutoLabelResult, bool, error) {
	if phygoboost.InWorker() {
		return blastAutoLabelResult{}, false, nil
	}
	var output autoIdentifyBlastLabelOutput
	err := phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "phytozome-next.jgi.doe.gov",
		Description: "auto identify blast label",
	}, autoIdentifyBlastLabelWorker, autoIdentifyBlastLabelInput{
		Selected: selected,
		Item:     item,
	}, &output)
	return output.Result, true, err
}


