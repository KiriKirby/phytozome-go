package workflow

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"
)

var (
	managedHTTPClient    = phygoboost.HTTPClient()
	managedWizardCacheMu sync.Mutex
	managedWizardCache   = make(map[string]*BlastWizard)
)

func referenceManagedTaskSpec(domain string, description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      domain,
		Description: description,
	}
}

func managedWizardForDatabase(database string) (*BlastWizard, error) {
	database = strings.ToLower(strings.TrimSpace(database))
	if database == "" {
		return nil, nil
	}
	managedWizardCacheMu.Lock()
	defer managedWizardCacheMu.Unlock()
	if wizard, ok := managedWizardCache[database]; ok && wizard != nil {
		return wizard, nil
	}
	seed := &BlastWizard{httpClient: managedHTTPClient}
	src, err := seed.dataSourceForDatabase(database)
	if err != nil {
		return nil, err
	}
	wizard := &BlastWizard{
		source:                    src,
		httpClient:                managedHTTPClient,
		speciesCandidatesCache:    make(map[string][]model.SpeciesCandidate),
		proteinSequenceCache:      make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:       make(map[string]error),
		rowUniProtAccessionsCache: make(map[string][]string),
		rowUniProtAccessionsKnown: make(map[string]bool),
		uniProtLookupCache:        make(map[string]uniProtLookupResult),
		interProLookupCache:       make(map[string]interProLookupResult),
	}
	managedWizardCache[database] = wizard
	return wizard, nil
}

func (w *BlastWizard) lookupUniProtEntryManaged(ctx context.Context, row model.BlastResultRow, accessions []string) (uniprot.Entry, bool, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return uniprot.Entry{}, false, false, nil
	}
	var (
		entry uniprot.Entry
		ok    bool
	)
	err := phygoboost.RunWithTimeout(ctx, 45*time.Second, func(runCtx context.Context) error {
		return phygoboost.RunTaskSpec(runCtx, referenceManagedTaskSpec("rest.uniprot.org", "lookup uniprot entry"), func(taskCtx context.Context) error {
			if len(accessions) > 0 && strings.TrimSpace(row.UniProtAccession) == "" {
				row.UniProtAccession = strings.TrimSpace(accessions[0])
			}
			entry, ok, _ = w.lookupUniProtEntry(taskCtx, w.sharedUniProtClient(), row)
			return nil
		})
	})
	return entry, ok, true, err
}

func (w *BlastWizard) lookupInterProEntryManaged(ctx context.Context, row model.BlastResultRow, accessions []string) (interpro.Entry, bool, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return interpro.Entry{}, false, false, nil
	}
	var (
		entry interpro.Entry
		ok    bool
	)
	err := phygoboost.RunWithTimeout(ctx, 45*time.Second, func(runCtx context.Context) error {
		return phygoboost.RunTaskSpec(runCtx, referenceManagedTaskSpec("www.ebi.ac.uk", "lookup interpro entry"), func(taskCtx context.Context) error {
			if len(accessions) > 0 && strings.TrimSpace(row.UniProtAccession) == "" {
				row.UniProtAccession = strings.TrimSpace(accessions[0])
			}
			var err error
			entry, ok, err = w.lookupInterProEntry(taskCtx, w.sharedInterProClient(), row)
			return err
		})
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		return interpro.Entry{}, false, true, context.DeadlineExceeded
	}
	return entry, ok, true, err
}
