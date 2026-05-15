package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/workflow"
)

func TestParseLaunchArgsLoadsHandoffDefaults(t *testing.T) {
	tmp := t.TempDir()
	handoffPath := filepath.Join(tmp, "handoff.json")
	handoff := workflow.InstanceHandoff{
		Kind:       "blast-session",
		RunID:      "run-123",
		ParentID:   "2",
		InstanceID: "2.1",
		Database:   "phytozome",
		Mode:       "blast",
	}
	data, err := json.Marshal(handoff)
	if err != nil {
		t.Fatalf("marshal handoff: %v", err)
	}
	if err := os.WriteFile(handoffPath, data, 0o600); err != nil {
		t.Fatalf("write handoff: %v", err)
	}

	launch, args, err := parseLaunchArgs([]string{"--handoff", handoffPath, "blast", "wizard"})
	if err != nil {
		t.Fatalf("parse launch args: %v", err)
	}
	if len(args) != 2 || args[0] != "blast" || args[1] != "wizard" {
		t.Fatalf("args = %#v, want [blast wizard]", args)
	}
	if launch.RunID != "run-123" {
		t.Fatalf("launch.RunID = %q, want run-123", launch.RunID)
	}
	if launch.InstanceID != "2.1" {
		t.Fatalf("launch.InstanceID = %q, want 2.1", launch.InstanceID)
	}
	if launch.ParentInstanceID != "2" {
		t.Fatalf("launch.ParentInstanceID = %q, want 2", launch.ParentInstanceID)
	}
	if launch.Database != "phytozome" {
		t.Fatalf("launch.Database = %q, want phytozome", launch.Database)
	}
	if launch.Mode != workflow.ModeBlast {
		t.Fatalf("launch.Mode = %q, want %q", launch.Mode, workflow.ModeBlast)
	}
}
