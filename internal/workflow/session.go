// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

type sessionManifest struct {
	RunID         string                     `json:"run_id"`
	CreatedAt     time.Time                  `json:"created_at"`
	GlobalRunRoot bool                       `json:"global_run_root"`
	Instances     map[string]sessionInstance `json:"instances"`
	NextOrdinals  map[string]int             `json:"next_ordinals,omitempty"`
}

type sessionInstance struct {
	InstanceID string    `json:"instance_id"`
	ParentID   string    `json:"parent_id"`
	Kind       string    `json:"kind"`
	UpdatedAt  time.Time `json:"updated_at"`
}

var sessionMu sync.Mutex

func sessionRoot(runID string) (string, error) {
	return appfs.CacheDir("session", strings.TrimSpace(runID))
}

func sessionManifestPath(runID string) (string, error) {
	root, err := sessionRoot(runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "manifest.json"), nil
}

func loadSessionManifest(runID string) (sessionManifest, error) {
	path, err := sessionManifestPath(runID)
	if err != nil {
		return sessionManifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sessionManifest{
				RunID:     runID,
				CreatedAt: time.Now().UTC(),
				Instances: map[string]sessionInstance{},
			}, nil
		}
		return sessionManifest{}, fmt.Errorf("read session manifest: %w", err)
	}
	var manifest sessionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return sessionManifest{}, fmt.Errorf("decode session manifest: %w", err)
	}
	if manifest.Instances == nil {
		manifest.Instances = map[string]sessionInstance{}
	}
	if manifest.NextOrdinals == nil {
		manifest.NextOrdinals = map[string]int{}
	}
	for key, inst := range manifest.Instances {
		inst.InstanceID = strings.TrimSpace(inst.InstanceID)
		if inst.InstanceID == "" {
			inst.InstanceID = strings.TrimSpace(key)
		}
		manifest.Instances[inst.InstanceID] = inst
		if inst.InstanceID != key {
			delete(manifest.Instances, key)
		}
	}
	return manifest, nil
}

func saveSessionManifest(runID string, manifest sessionManifest) error {
	path, err := sessionManifestPath(runID)
	if err != nil {
		return err
	}
	root := filepath.Dir(path)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("ensure session root: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session manifest: %w", err)
	}
	tmp, err := os.CreateTemp(root, ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("create session manifest temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write session manifest temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session manifest temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace session manifest: %w", err)
	}
	return nil
}

func allocateSessionInstance(runID string, parentID string, kind string) (sessionInstance, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	manifest, err := loadSessionManifest(runID)
	if err != nil {
		return sessionInstance{}, err
	}
	siblingIndex := nextSiblingOrdinal(manifest, strings.TrimSpace(parentID))
	instance := sessionInstance{
		ParentID:  strings.TrimSpace(parentID),
		Kind:      strings.TrimSpace(kind),
		UpdatedAt: time.Now().UTC(),
	}
	if instance.ParentID == "" {
		instance.InstanceID = fmt.Sprintf("%d", siblingIndex)
	} else {
		instance.InstanceID = instance.ParentID + "." + fmt.Sprintf("%d", siblingIndex)
	}
	manifest.Instances[instance.InstanceID] = instance
	if err := saveSessionManifest(runID, manifest); err != nil {
		return sessionInstance{}, err
	}
	return instance, nil
}

func newSessionRunID() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate session run id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func ensureLaunchSession(runID string, instanceID string, parentID string, kind string) (string, string, string, error) {
	runID = strings.TrimSpace(runID)
	instanceID = strings.TrimSpace(instanceID)
	parentID = strings.TrimSpace(parentID)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "blast"
	}
	if runID == "" {
		generated, err := newSessionRunID()
		if err != nil {
			return "", "", "", err
		}
		runID = generated
	}
	if instanceID == "" {
		inst, err := allocateSessionInstance(runID, parentID, kind)
		if err != nil {
			return "", "", "", err
		}
		if strings.TrimSpace(parentID) == "" {
			_ = markGlobalRunRoot(runID)
		}
		return runID, inst.InstanceID, inst.ParentID, nil
	}
	return runID, instanceID, parentID, nil
}

func allocateRootSessionInstance(runID string, kind string) (sessionInstance, error) {
	inst, err := allocateSessionInstance(runID, "", kind)
	if err != nil {
		return sessionInstance{}, err
	}
	if err := markGlobalRunRoot(runID); err != nil {
		return sessionInstance{}, err
	}
	return inst, nil
}

func markGlobalRunRoot(runID string) error {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	manifest, err := loadSessionManifest(runID)
	if err != nil {
		return err
	}
	manifest.GlobalRunRoot = true
	return saveSessionManifest(runID, manifest)
}

func nextSiblingOrdinal(manifest sessionManifest, parentID string) int {
	if manifest.NextOrdinals == nil {
		manifest.NextOrdinals = map[string]int{}
	}
	key := strings.TrimSpace(parentID)
	if next := manifest.NextOrdinals[key]; next > 0 {
		manifest.NextOrdinals[key] = next + 1
		return next
	}
	maxOrdinal := 0
	prefix := ""
	if parentID != "" {
		prefix = parentID + "."
	}
	for _, inst := range manifest.Instances {
		if strings.TrimSpace(inst.ParentID) != parentID {
			continue
		}
		id := strings.TrimSpace(inst.InstanceID)
		if id == "" {
			continue
		}
		part := id
		if prefix != "" {
			part = strings.TrimPrefix(id, prefix)
		}
		if dot := strings.Index(part, "."); dot >= 0 {
			part = part[:dot]
		}
		if n, err := strconv.Atoi(part); err == nil && n > maxOrdinal {
			maxOrdinal = n
		}
	}
	next := maxOrdinal + 1
	manifest.NextOrdinals[key] = next + 1
	return next
}

func (w *BlastWizard) markInstanceActive() error {
	if strings.TrimSpace(w.instanceRunID) == "" || strings.TrimSpace(w.instanceID) == "" {
		return nil
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	manifest, err := loadSessionManifest(w.instanceRunID)
	if err != nil {
		return err
	}
	inst := manifest.Instances[w.instanceID]
	inst.InstanceID = w.instanceID
	inst.ParentID = w.parentInstanceID
	inst.Kind = "blast"
	inst.UpdatedAt = time.Now().UTC()
	manifest.Instances[w.instanceID] = inst
	return saveSessionManifest(w.instanceRunID, manifest)
}

func (w *BlastWizard) markInstanceInactive() error {
	if strings.TrimSpace(w.instanceRunID) == "" || strings.TrimSpace(w.instanceID) == "" {
		return nil
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	manifest, err := loadSessionManifest(w.instanceRunID)
	if err != nil {
		return err
	}
	delete(manifest.Instances, w.instanceID)
	return saveSessionManifest(w.instanceRunID, manifest)
}
