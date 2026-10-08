package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/viewersnapshot"
)

func TestCanvasTreeArtifactDirUsesCacheRoot(t *testing.T) {
	root, err := appfs.CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot returned error: %v", err)
	}
	got, err := canvasTreeArtifactDir("canvas/session", "run:1")
	if err != nil {
		t.Fatalf("resolve Canvas tree cache: %v", err)
	}
	want := filepath.Join(root, "tree", "canvas_session", "run_1")
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(want)) {
		t.Fatalf("canvasTreeArtifactDir = %q, want %q", got, want)
	}
}

func TestLoadNewickBrowserTextFromLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.nwk")
	if err := os.WriteFile(path, []byte("(A:0.1,B:0.2);\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	got, err := loadNewickBrowserText(context.Background(), nil, path)
	if err != nil {
		t.Fatalf("loadNewickBrowserText returned error: %v", err)
	}
	if got != "(A:0.1,B:0.2);" {
		t.Fatalf("loadNewickBrowserText = %q", got)
	}
}

func TestLoadNewickBrowserTextFromRemoteURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tree.nwk" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("(A,B,(C,D));"))
	}))
	defer server.Close()

	got, err := loadNewickBrowserText(context.Background(), server.Client(), server.URL+"/tree.nwk")
	if err != nil {
		t.Fatalf("loadNewickBrowserText returned error: %v", err)
	}
	if got != "(A,B,(C,D));" {
		t.Fatalf("loadNewickBrowserText = %q", got)
	}
}

func TestLoadNewickBrowserTextRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(txtPath, []byte("(A,B);"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	tests := []struct {
		name   string
		target string
	}{
		{name: "multiline", target: "one\nline"},
		{name: "wrong extension", target: txtPath},
		{name: "unsupported scheme", target: "ftp://example.com/tree.nwk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadNewickBrowserText(context.Background(), nil, tt.target); err == nil {
				t.Fatalf("loadNewickBrowserText(%q) returned nil error", tt.target)
			}
		})
	}
}

func TestLoadNewickBrowserPayloadBuildsViewerPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.nwk")
	if err := os.WriteFile(path, []byte("(Alpha,Beta);"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	payload, err := loadNewickBrowserPayload(context.Background(), nil, "session-1", path, now)
	if err != nil {
		t.Fatalf("loadNewickBrowserPayload returned error: %v", err)
	}
	if payload.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", payload.SessionID)
	}
	if payload.Newick != "(Alpha,Beta);" {
		t.Fatalf("Newick = %q", payload.Newick)
	}
	if payload.Title != "sample.nwk" {
		t.Fatalf("Title = %q, want source filename", payload.Title)
	}
	if !payload.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", payload.UpdatedAt, now)
	}
}

func TestLoadTreeBrowserSessionFromPGV(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 30, 12, 30, 0, 0, time.UTC)
	snapshot := viewersnapshot.New(phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "old-session",
		UpdatedAt:     now,
		Newick:        "(A,B);",
		Metadata: phylo.Metadata{
			SchemaVersion: 1,
			GeneratedAt:   now,
		},
	}, []byte(`{"schema_version":3,"reactree":{"schema_version":3,"layout":"circular","renderStyle":"mega","exportLongEdge":8192,"hScale":0},"phgo":{"split_percent":42}}`), now)
	data, err := viewersnapshot.Encode(snapshot)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	path := filepath.Join(dir, "sample.pgv")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	payload, state, err := loadTreeBrowserSession(context.Background(), nil, "new-session", path, now)
	if err != nil {
		t.Fatalf("loadTreeBrowserSession returned error: %v", err)
	}
	if payload.SessionID != "new-session" {
		t.Fatalf("SessionID = %q", payload.SessionID)
	}
	if payload.Newick != "(A,B);" {
		t.Fatalf("Newick = %q", payload.Newick)
	}
	if payload.Title != "sample.pgv" {
		t.Fatalf("Title = %q, want opened PGV filename", payload.Title)
	}
	if !strings.Contains(string(state), `"layout": "circular"`) {
		t.Fatalf("viewer state was not preserved: %s", state)
	}
	if !strings.Contains(string(state), `"exportLongEdge": 8192`) || !strings.Contains(string(state), `"hScale": 0`) {
		t.Fatalf("viewer v3 state was not preserved: %s", state)
	}
}

func TestPutViewerPayloadPublishesSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := phylo.NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	now := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	payload := phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "browser-test",
		UpdatedAt:     now,
		Newick:        "(A,B);",
		Metadata: phylo.Metadata{
			SchemaVersion: 1,
			GeneratedAt:   now,
		},
	}
	if err := putViewerPayload(context.Background(), server, "browser-test", payload); err != nil {
		t.Fatalf("putViewerPayload returned error: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/sessions/%s/payload", server.URL(), "browser-test"))
	if err != nil {
		t.Fatalf("payload GET returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("payload GET status = %d", resp.StatusCode)
	}
}

func TestPutCanvasTreeViewerPayloadUsesProgramTabTitle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := phylo.NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	w := &BlastWizard{instanceID: "2.3"}
	payload := phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "old-session",
		Title:         "System tree",
		UpdatedAt:     time.Now(),
		Newick:        "(A,B);",
	}
	if err := w.putCanvasTreeViewerPayload(ctx, server, payload); err != nil {
		t.Fatalf("putCanvasTreeViewerPayload returned error: %v", err)
	}

	resp, err := http.Get(server.URL() + "/sessions/2.3/payload")
	if err != nil {
		t.Fatalf("payload GET failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"title":"2.3"`) {
		t.Fatalf("payload title = %s, want program tab title", body)
	}
}
