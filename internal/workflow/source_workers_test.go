package workflow

import (
	"net/http"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/source"
)

func TestSourceProcessDatabaseRecognizesBuiltInSources(t *testing.T) {
	tests := []struct {
		name string
		src  source.DataSource
		want string
	}{
		{name: "phytozome", src: phytozome.NewClient(nil), want: "phytozome"},
		{name: "lemna", src: lemna.NewClient(nil), want: "lemna"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceProcessDatabase(tt.src); got != tt.want {
				t.Fatalf("sourceProcessDatabase(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSourceProcessDomainRecognizesBuiltInSources(t *testing.T) {
	tests := []struct {
		name     string
		database string
		want     string
	}{
		{name: "phytozome", database: "phytozome", want: "phytozome-next.jgi.doe.gov"},
		{name: "lemna", database: "lemna", want: "www.lemna.org"},
		{name: "blank", database: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceProcessDomain(tt.database); got != tt.want {
				t.Fatalf("sourceProcessDomain(%q) = %q, want %q", tt.database, got, tt.want)
			}
		})
	}
}

func TestSourceWorkerTaskSpecUsesHeavyAndRealDomain(t *testing.T) {
	spec := sourceWorkerTaskSpec("phytozome", "fetch species candidates")
	if spec.Level != phygoboost.ExecHeavy {
		t.Fatalf("sourceWorkerTaskSpec level = %v, want ExecHeavy", spec.Level)
	}
	if spec.Domain != "phytozome-next.jgi.doe.gov" {
		t.Fatalf("sourceWorkerTaskSpec domain = %q, want phytozome-next.jgi.doe.gov", spec.Domain)
	}
	if spec.Description != "fetch species candidates" {
		t.Fatalf("sourceWorkerTaskSpec description = %q", spec.Description)
	}
}

func TestSourceWorkerTaskSpecLeavesLocalSlotsToZeroForNetworkStages(t *testing.T) {
	spec := sourceWorkerTaskSpec("lemna", "fetch protein sequence")
	if spec.LocalSlots != 0 {
		t.Fatalf("sourceWorkerTaskSpec local slots = %d, want 0 for pure source network bridge", spec.LocalSlots)
	}
	if spec.Domain != "www.lemna.org" {
		t.Fatalf("sourceWorkerTaskSpec domain = %q, want www.lemna.org", spec.Domain)
	}
}

func TestResolverForQuerySourceCarriesWorkerDatabaseName(t *testing.T) {
	resolver, database, ok := resolverForQuerySource(&model.QuerySequenceSource{SourceDatabase: "phytozome"}, http.DefaultClient)
	if !ok {
		t.Fatal("resolverForQuerySource did not recognize phytozome source")
	}
	if resolver == nil {
		t.Fatal("resolverForQuerySource returned nil resolver")
	}
	if database != "phytozome" {
		t.Fatalf("resolver database = %q, want phytozome", database)
	}
}

func TestFetchSpeciesCandidatesProcessUsesWorkerBridge(t *testing.T) {
	w := &BlastWizard{source: phytozome.NewClient(nil)}
	_, handled, _ := w.fetchSpeciesCandidatesProcess(nil, w.source)
	if !handled {
		t.Fatal("fetchSpeciesCandidatesProcess should use worker bridge")
	}
}

func TestRemoteBlastWorkerPathsUseSplitBridges(t *testing.T) {
	w := &BlastWizard{source: phytozome.NewClient(nil)}
	if _, handled, err := w.submitBlastProcess(nil, model.BlastRequest{Program: "BLASTP"}); !handled || err != nil {
		t.Fatalf("submitBlastProcess remote = handled %v err %v, want handled true nil err", handled, err)
	}
	if _, _, handled, _ := w.runBlastProcess(nil, model.BlastRequest{Program: "BLASTP"}, 0, 0); !handled {
		t.Fatal("runBlastProcess should use worker bridge for remote phytozome requests")
	}
	if _, handled, err := w.waitBlastResultsProcess(nil, "remote-job", 0, 0); !handled || err != nil {
		t.Fatalf("waitBlastResultsProcess remote = handled %v err %v, want handled true nil err", handled, err)
	}
}

func TestSourceSubmitBlastTaskSpecUsesLocalSlotForLocalRequests(t *testing.T) {
	spec := sourceSubmitBlastTaskSpec("lemna", model.BlastRequest{Program: "local:BLASTP"})
	if spec.Level != phygoboost.ExecHeavy {
		t.Fatalf("level = %v, want ExecHeavy", spec.Level)
	}
	if spec.LocalSlots != 1 {
		t.Fatalf("local slots = %d, want 1", spec.LocalSlots)
	}
	if spec.Domain != "www.lemna.org" {
		t.Fatalf("domain = %q, want www.lemna.org", spec.Domain)
	}
}

func TestSourceWaitBlastTaskSpecUsesNetworkForRemoteJobs(t *testing.T) {
	spec := sourceWaitBlastTaskSpec("phytozome", "remote-job")
	if spec.LocalSlots != 0 {
		t.Fatalf("local slots = %d, want 0", spec.LocalSlots)
	}
	if spec.Domain != "phytozome-next.jgi.doe.gov" {
		t.Fatalf("domain = %q, want phytozome-next.jgi.doe.gov", spec.Domain)
	}
}

func TestSourceWaitBlastTaskSpecUsesLocalSlotsForLocalJobs(t *testing.T) {
	spec := sourceWaitBlastTaskSpec("lemna", "local-job-123")
	if spec.LocalSlots != 1 {
		t.Fatalf("local slots = %d, want 1", spec.LocalSlots)
	}
	if spec.Domain != "" {
		t.Fatalf("domain = %q, want empty local wait domain", spec.Domain)
	}
}

func TestDetectLemnaBlastCapabilitiesProcessUsesWorkerBridge(t *testing.T) {
	if _, handled, _ := detectLemnaBlastCapabilitiesProcess(nil, model.SpeciesCandidate{}); !handled {
		t.Fatal("detectLemnaBlastCapabilitiesProcess should use worker bridge")
	}
}
