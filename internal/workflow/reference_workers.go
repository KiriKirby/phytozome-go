package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"
	"github.com/goccy/go-json"
)

const (
	lookupUniProtEntryWorker  = "workflow.reference.lookup_uniprot_entry"
	lookupUniProtBatchWorker  = "workflow.reference.lookup_uniprot_batch"
	lookupInterProEntryWorker = "workflow.reference.lookup_interpro_entry"
	lookupInterProBatchWorker = "workflow.reference.lookup_interpro_batch"
)

var (
	workerHTTPClient    = phygoboost.HTTPClient()
	workerWizardCacheMu sync.Mutex
	workerWizardCache   = make(map[string]*BlastWizard)
)

func referenceWorkerTaskSpec(domain string, description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Domain:      domain,
		Description: description,
	}
}

type lookupUniProtEntryInput struct {
	Database   string               `json:"database"`
	Row        model.BlastResultRow `json:"row"`
	Accessions []string             `json:"accessions,omitempty"`
}

type lookupUniProtEntryOutput struct {
	Entry uniprot.Entry `json:"entry"`
	OK    bool          `json:"ok"`
}

type lookupUniProtBatchInput struct {
	Database   string                 `json:"database"`
	Rows       []model.BlastResultRow `json:"rows"`
	Accessions [][]string             `json:"accessions,omitempty"`
}

type lookupUniProtBatchOutput struct {
	Results []lookupUniProtEntryOutput `json:"results"`
}

type lookupInterProEntryInput struct {
	Database   string               `json:"database"`
	Row        model.BlastResultRow `json:"row"`
	Accessions []string             `json:"accessions,omitempty"`
}

type lookupInterProEntryOutput struct {
	Entry interpro.Entry `json:"entry"`
	OK    bool           `json:"ok"`
}

type lookupInterProBatchInput struct {
	Database   string                 `json:"database"`
	Rows       []model.BlastResultRow `json:"rows"`
	Accessions [][]string             `json:"accessions,omitempty"`
}

type lookupInterProBatchOutput struct {
	Results []lookupInterProEntryOutput `json:"results"`
}

func init() {
	phygoboost.Register(lookupUniProtEntryWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input lookupUniProtEntryInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode UniProt lookup input: %w", err)
		}
		wizard, err := workerWizardForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		entry, ok := wizard.lookupUniProtEntryWithAccessions(ctx, wizard.uniprotReferenceClient(), input.Row, input.Accessions)
		return json.Marshal(lookupUniProtEntryOutput{Entry: entry, OK: ok})
	})
	phygoboost.Register(lookupInterProEntryWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input lookupInterProEntryInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode InterPro lookup input: %w", err)
		}
		wizard, err := workerWizardForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		entry, ok, err := wizard.lookupInterProEntryWithAccessions(ctx, wizard.interproReferenceClient(), input.Row, input.Accessions)
		if err != nil {
			return nil, err
		}
		return json.Marshal(lookupInterProEntryOutput{Entry: entry, OK: ok})
	})
	phygoboost.Register(lookupUniProtBatchWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input lookupUniProtBatchInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode UniProt batch lookup input: %w", err)
		}
		wizard, err := workerWizardForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		client := wizard.uniprotReferenceClient()
		results := make([]lookupUniProtEntryOutput, len(input.Rows))
		for i := range input.Rows {
			var accessions []string
			if i < len(input.Accessions) {
				accessions = input.Accessions[i]
			}
			entry, ok := wizard.lookupUniProtEntryWithAccessions(ctx, client, input.Rows[i], accessions)
			results[i] = lookupUniProtEntryOutput{Entry: entry, OK: ok}
		}
		return json.Marshal(lookupUniProtBatchOutput{Results: results})
	})
	phygoboost.Register(lookupInterProBatchWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input lookupInterProBatchInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode InterPro batch lookup input: %w", err)
		}
		wizard, err := workerWizardForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		client := wizard.interproReferenceClient()
		results := make([]lookupInterProEntryOutput, len(input.Rows))
		for i := range input.Rows {
			var accessions []string
			if i < len(input.Accessions) {
				accessions = input.Accessions[i]
			}
			entry, ok, err := wizard.lookupInterProEntryWithAccessions(ctx, client, input.Rows[i], accessions)
			if err != nil {
				return nil, err
			}
			results[i] = lookupInterProEntryOutput{Entry: entry, OK: ok}
		}
		return json.Marshal(lookupInterProBatchOutput{Results: results})
	})
}

func workerWizardForDatabase(database string) (*BlastWizard, error) {
	database = strings.ToLower(strings.TrimSpace(database))
	if database == "" {
		return nil, fmt.Errorf("database is empty")
	}
	workerWizardCacheMu.Lock()
	defer workerWizardCacheMu.Unlock()
	if wizard, ok := workerWizardCache[database]; ok && wizard != nil {
		return wizard, nil
	}
	src, err := dataSourceForName(database, workerHTTPClient)
	if err != nil {
		return nil, err
	}
	wizard := &BlastWizard{
		source:                 src,
		httpClient:             workerHTTPClient,
		speciesCandidatesCache: make(map[string][]model.SpeciesCandidate),
		phytozomeTargetCache:   make(map[string]int),
		proteinSequenceCache:   make(map[string]string),
		proteinSequenceMiss:    make(map[string]error),
		uniProtAccessions:      make(map[string][]string),
		interProQueryCache:     make(map[string]cachedInterProQueryEntry),
	}
	workerWizardCache[database] = wizard
	return wizard, nil
}

func (w *BlastWizard) lookupUniProtEntryProcess(ctx context.Context, row model.BlastResultRow, accessions []string) (uniprot.Entry, bool, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return uniprot.Entry{}, false, false, nil
	}
	var output lookupUniProtEntryOutput
	err := phygoboost.RunWithTimeout(ctx, proteinFetchTimeout, func(runCtx context.Context) error {
		return phygoboost.RunTaskJSON(runCtx, referenceWorkerTaskSpec("rest.uniprot.org", "lookup uniprot entry"), lookupUniProtEntryWorker, lookupUniProtEntryInput{
			Database:   database,
			Row:        row,
			Accessions: accessions,
		}, &output)
	})
	return output.Entry, output.OK, true, err
}

func (w *BlastWizard) lookupInterProEntryProcess(ctx context.Context, row model.BlastResultRow, accessions []string) (interpro.Entry, bool, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return interpro.Entry{}, false, false, nil
	}
	var output lookupInterProEntryOutput
	err := phygoboost.RunWithTimeout(ctx, proteinFetchTimeout, func(runCtx context.Context) error {
		return phygoboost.RunTaskJSON(runCtx, referenceWorkerTaskSpec("www.ebi.ac.uk", "lookup interpro entry"), lookupInterProEntryWorker, lookupInterProEntryInput{
			Database:   database,
			Row:        row,
			Accessions: accessions,
		}, &output)
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		return interpro.Entry{}, false, true, context.DeadlineExceeded
	}
	return output.Entry, output.OK, true, err
}

func (w *BlastWizard) lookupUniProtEntriesProcess(ctx context.Context, rows []model.BlastResultRow, accessions [][]string) ([]lookupUniProtEntryOutput, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() || len(rows) == 0 {
		return nil, false, nil
	}
	var output lookupUniProtBatchOutput
	err := phygoboost.RunWithTimeout(ctx, proteinFetchTimeout, func(runCtx context.Context) error {
		return phygoboost.RunTaskJSON(runCtx, referenceWorkerTaskSpec("rest.uniprot.org", "lookup uniprot batch"), lookupUniProtBatchWorker, lookupUniProtBatchInput{
			Database:   database,
			Rows:       rows,
			Accessions: accessions,
		}, &output)
	})
	return output.Results, true, err
}

func (w *BlastWizard) lookupInterProEntriesProcess(ctx context.Context, rows []model.BlastResultRow, accessions [][]string) ([]lookupInterProEntryOutput, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() || len(rows) == 0 {
		return nil, false, nil
	}
	var output lookupInterProBatchOutput
	err := phygoboost.RunWithTimeout(ctx, proteinFetchTimeout, func(runCtx context.Context) error {
		return phygoboost.RunTaskJSON(runCtx, referenceWorkerTaskSpec("www.ebi.ac.uk", "lookup interpro batch"), lookupInterProBatchWorker, lookupInterProBatchInput{
			Database:   database,
			Rows:       rows,
			Accessions: accessions,
		}, &output)
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		return nil, true, context.DeadlineExceeded
	}
	return output.Results, true, err
}
