package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureLaunchSessionAllocatesRootAndChildren(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	runID, rootID, parentID, err := ensureLaunchSession("", "", "", "blast")
	if err != nil {
		t.Fatalf("ensure root session: %v", err)
	}
	if runID == "" {
		t.Fatal("runID is empty")
	}
	if rootID != "1" {
		t.Fatalf("root instanceID = %q, want 1", rootID)
	}
	if parentID != "" {
		t.Fatalf("root parentID = %q, want empty", parentID)
	}

	runID2, childID, childParentID, err := ensureLaunchSession(runID, "", rootID, "blast")
	if err != nil {
		t.Fatalf("ensure child session: %v", err)
	}
	if runID2 != runID {
		t.Fatalf("child runID = %q, want %q", runID2, runID)
	}
	if childID != "1.1" {
		t.Fatalf("child instanceID = %q, want 1.1", childID)
	}
	if childParentID != "1" {
		t.Fatalf("child parentID = %q, want 1", childParentID)
	}

	manifestPath := filepath.Join(tmp, ".cache", "session", runID, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

func TestAllocateSessionInstanceUsesHierarchicalSiblingCounts(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	runID := "run-hierarchy"
	root1, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate root1: %v", err)
	}
	root2, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate root2: %v", err)
	}
	child21, err := allocateSessionInstance(runID, root2.InstanceID, "blast")
	if err != nil {
		t.Fatalf("allocate child21: %v", err)
	}
	child22, err := allocateSessionInstance(runID, root2.InstanceID, "blast")
	if err != nil {
		t.Fatalf("allocate child22: %v", err)
	}
	child221, err := allocateSessionInstance(runID, child22.InstanceID, "blast")
	if err != nil {
		t.Fatalf("allocate child221: %v", err)
	}

	if root1.InstanceID != "1" {
		t.Fatalf("root1 = %q, want 1", root1.InstanceID)
	}
	if root2.InstanceID != "2" {
		t.Fatalf("root2 = %q, want 2", root2.InstanceID)
	}
	if child21.InstanceID != "2.1" {
		t.Fatalf("child21 = %q, want 2.1", child21.InstanceID)
	}
	if child22.InstanceID != "2.2" {
		t.Fatalf("child22 = %q, want 2.2", child22.InstanceID)
	}
	if child221.InstanceID != "2.2.1" {
		t.Fatalf("child221 = %q, want 2.2.1", child221.InstanceID)
	}
}

func TestEnsureLaunchSessionPreservesProvidedInstance(t *testing.T) {
	runID, instanceID, parentID, err := ensureLaunchSession("run-existing", "2.3", "2", "blast")
	if err != nil {
		t.Fatalf("ensure existing session: %v", err)
	}
	if runID != "run-existing" {
		t.Fatalf("runID = %q, want run-existing", runID)
	}
	if instanceID != "2.3" {
		t.Fatalf("instanceID = %q, want 2.3", instanceID)
	}
	if parentID != "2" {
		t.Fatalf("parentID = %q, want 2", parentID)
	}
}

func TestAllocateSessionInstanceReusesReleasedSiblingOrdinal(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	runID := "run-reuse-child"
	root, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate root: %v", err)
	}
	child, err := allocateSessionInstance(runID, root.InstanceID, "blast")
	if err != nil {
		t.Fatalf("allocate child: %v", err)
	}

	wizard := &BlastWizard{
		instanceRunID: runID,
		instanceID:    child.InstanceID,
	}
	if err := wizard.markInstanceInactive(); err != nil {
		t.Fatalf("mark child inactive: %v", err)
	}

	reused, err := allocateSessionInstance(runID, root.InstanceID, "blast")
	if err != nil {
		t.Fatalf("allocate reused child: %v", err)
	}
	if reused.InstanceID != "1.1" {
		t.Fatalf("reused child = %q, want 1.1", reused.InstanceID)
	}
}

func TestAllocateRootSessionInstanceReusesReleasedRootOrdinal(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	runID := "run-reuse-root"
	root1, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate root1: %v", err)
	}
	root2, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate root2: %v", err)
	}

	wizard := &BlastWizard{
		instanceRunID: runID,
		instanceID:    root2.InstanceID,
	}
	if err := wizard.markInstanceInactive(); err != nil {
		t.Fatalf("mark root2 inactive: %v", err)
	}

	reused, err := allocateSessionInstance(runID, "", "blast")
	if err != nil {
		t.Fatalf("allocate reused root: %v", err)
	}
	if reused.InstanceID != "2" {
		t.Fatalf("reused root = %q, want 2", reused.InstanceID)
	}
	if root1.InstanceID != "1" {
		t.Fatalf("root1 changed unexpectedly: %q", root1.InstanceID)
	}
}
