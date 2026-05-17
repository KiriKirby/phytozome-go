package workflow

import (
	"net/http"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tair"
)

func TestSourceDatabaseNameRecognizesBuiltInSources(t *testing.T) {
	tests := []struct {
		name string
		src  source.DataSource
		want string
	}{
		{name: "phytozome", src: phytozome.NewClient(nil), want: "phytozome"},
		{name: "lemna", src: lemna.NewClient(nil), want: "lemna"},
		{name: "tair", src: tair.NewClient(nil), want: "tair"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceDatabaseName(tt.src); got != tt.want {
				t.Fatalf("sourceDatabaseName(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSourceDomainRecognizesBuiltInSources(t *testing.T) {
	tests := []struct {
		name     string
		database string
		want     string
	}{
		{name: "phytozome", database: "phytozome", want: "phytozome-next.jgi.doe.gov"},
		{name: "lemna", database: "lemna", want: "www.lemna.org"},
		{name: "tair", database: "tair", want: "www.arabidopsis.org"},
		{name: "blank", database: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceDomain(tt.database); got != tt.want {
				t.Fatalf("sourceDomain(%q) = %q, want %q", tt.database, got, tt.want)
			}
		})
	}
}

func TestSourceManagedTaskSpecUsesManagedAndRealDomain(t *testing.T) {
	spec := sourceManagedTaskSpec("phytozome", "fetch species candidates")
	if spec.Level != phygoboost.ExecManaged {
		t.Fatalf("sourceManagedTaskSpec level = %v, want ExecManaged", spec.Level)
	}
	if spec.Domain != "phytozome-next.jgi.doe.gov" {
		t.Fatalf("sourceManagedTaskSpec domain = %q, want phytozome-next.jgi.doe.gov", spec.Domain)
	}
	if spec.Description != "fetch species candidates" {
		t.Fatalf("sourceManagedTaskSpec description = %q", spec.Description)
	}
}

func TestSourceManagedTaskSpecLeavesDomainOnlyForNetworkStages(t *testing.T) {
	spec := sourceManagedTaskSpec("lemna", "fetch protein sequence")
	if spec.Domain != "www.lemna.org" {
		t.Fatalf("sourceManagedTaskSpec domain = %q, want www.lemna.org", spec.Domain)
	}
}

func TestResolverForQuerySourceCarriesManagedDatabaseName(t *testing.T) {
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

func TestRemoteBlastPathsStayManaged(t *testing.T) {
	w := &BlastWizard{source: phytozome.NewClient(nil)}
	if _, handled, err := w.submitBlastManaged(nil, model.BlastRequest{Program: "BLASTP"}); !handled {
		t.Fatalf("submitBlastManaged remote = handled %v err %v, want handled true", handled, err)
	}
	if _, _, handled, _ := w.runBlastManaged(nil, model.BlastRequest{Program: "BLASTP"}, 0, 0); !handled {
		t.Fatal("runBlastManaged should stay on the managed path for remote phytozome requests")
	}
	if _, handled, err := w.waitBlastResultsManaged(nil, "remote-job", 0, 0); !handled {
		t.Fatalf("waitBlastResultsManaged remote = handled %v err %v, want handled true", handled, err)
	}
}

func TestSourceSubmitBlastTaskSpecUsesLocalSlotForLocalRequests(t *testing.T) {
	spec := sourceSubmitBlastTaskSpec("lemna", model.BlastRequest{Program: "local:BLASTP"})
	if spec.Level != phygoboost.ExecManaged {
		t.Fatalf("level = %v, want ExecManaged", spec.Level)
	}
	if spec.Domain != "" {
		t.Fatalf("domain = %q, want empty local submit domain", spec.Domain)
	}
}

func TestSourceWaitBlastTaskSpecUsesNetworkForRemoteJobs(t *testing.T) {
	spec := sourceWaitBlastTaskSpec("phytozome", "remote-job")
	if spec.Domain != "phytozome-next.jgi.doe.gov" {
		t.Fatalf("domain = %q, want phytozome-next.jgi.doe.gov", spec.Domain)
	}
}

func TestSourceWaitBlastTaskSpecUsesManagedOnlyForLocalJobs(t *testing.T) {
	spec := sourceWaitBlastTaskSpec("lemna", "local-job-123")
	if spec.Domain != "" {
		t.Fatalf("domain = %q, want empty local wait domain", spec.Domain)
	}
}

func TestDetectLemnaBlastCapabilitiesManagedUsesManagedPath(t *testing.T) {
	if _, handled, _ := detectLemnaBlastCapabilitiesManaged(nil, model.SpeciesCandidate{}); !handled {
		t.Fatal("detectLemnaBlastCapabilitiesManaged should use the managed path")
	}
}
