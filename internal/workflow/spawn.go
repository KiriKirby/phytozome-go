// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
)

type SpawnLaunchOptions struct {
	Database string
	Mode     QueryMode
	Handoff  InstanceHandoff
	ParentID string
	RunID    string
}

func SpawnNewTab(ctx context.Context, opts SpawnLaunchOptions) (string, error) {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		runID = "run"
	}
	parentID := strings.TrimSpace(opts.ParentID)
	instance, err := allocateSessionInstance(runID, parentID, string(opts.Mode))
	if err != nil {
		return "", err
	}
	opts.Handoff.RunID = runID
	opts.Handoff.ParentID = instance.ParentID
	opts.Handoff.InstanceID = instance.InstanceID
	opts.Handoff.Database = strings.TrimSpace(opts.Database)
	opts.Handoff.Mode = string(opts.Mode)
	path, err := SaveInstanceHandoff(runID, opts.Handoff)
	if err != nil {
		return "", err
	}
	appendSessionDebugLog("spawn_new_tab run_id=%q instance_id=%q parent_id=%q database=%q mode=%q handoff=%q", runID, instance.InstanceID, instance.ParentID, opts.Handoff.Database, opts.Handoff.Mode, path)
	if err := launchInstanceProcess(ctx, runID, instance.InstanceID, path); err != nil {
		return "", err
	}
	return instance.InstanceID, nil
}

func launchInstanceProcess(ctx context.Context, runID string, instanceID string, handoffPath string) error {
	launcher, err := weztermCLIPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, launcher, weztermSpawnArgs(runID, instanceID, handoffPath)...)
	cmd.Dir = filepath.Dir(launcher)
	cmd.Env = append(os.Environ(), "PHYTOZOME_GO_INSTANCE_ID="+instanceID, "PHYTOZOME_GO_RUN_ID="+runID)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = nil
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new tab: %w", err)
	}
	return nil
}

func LoadInstanceHandoff(path string) (InstanceHandoff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceHandoff{}, err
	}
	var handoff InstanceHandoff
	if err := json.Unmarshal(data, &handoff); err != nil {
		return InstanceHandoff{}, err
	}
	return handoff, nil
}

func instanceHandoffPathInCache(runID string, instanceID string) string {
	root, err := appfs.CacheDir("session", strings.TrimSpace(runID), "handoff")
	if err != nil {
		return ""
	}
	return filepath.Join(root, strings.TrimSpace(instanceID)+".json")
}

func weztermCLIPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return resolveWezTermCLIPathFromExecutable(exe)
}

func resolveWezTermCLIPathFromExecutable(exe string) (string, error) {
	dir := filepath.Dir(exe)
	candidates := make([]string, 0, 4)
	switch runtime.GOOS {
	case "windows":
		candidates = append(candidates,
			filepath.Join(dir, "wezterm-cli.bin"),
			filepath.Join(dir, "wezterm.bin"),
			filepath.Join(dir, "wezterm.exe"),
		)
	default:
		candidates = append(candidates,
			filepath.Join(dir, "wezterm-cli"),
			filepath.Join(dir, "wezterm"),
			filepath.Join(dir, "wezterm-gui"),
			filepath.Join(dir, "wezterm.AppImage"),
		)
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	switch runtime.GOOS {
	case "windows":
		if path, lookErr := exec.LookPath("wezterm.exe"); lookErr == nil {
			return path, nil
		}
		if path, lookErr := exec.LookPath("wezterm"); lookErr == nil {
			return path, nil
		}
	default:
		if path, lookErr := exec.LookPath("wezterm"); lookErr == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not locate wezterm cli next to executable or in PATH")
}

func weztermSpawnArgs(runID string, instanceID string, handoffPath string) []string {
	appPath, _ := os.Executable()
	appDir := filepath.Dir(appPath)
	args := []string{
		"cli", "spawn",
		"--cwd", appDir,
		"--",
		appPath,
	}
	if strings.TrimSpace(runID) != "" {
		args = append(args, "--instance-run-id", runID)
	}
	if strings.TrimSpace(instanceID) != "" {
		args = append(args, "--instance-id", instanceID)
	}
	if strings.TrimSpace(handoffPath) != "" {
		args = append(args, "--handoff", handoffPath)
	}
	return args
}

func syncInstanceTerminalMetadata(version string, runID string, instanceID string) {
	runID = strings.TrimSpace(runID)
	instanceID = strings.TrimSpace(instanceID)
	_ = setWezTermWindowTitle(windowTitle(version))
	if instanceID != "" {
		_ = setWezTermTabTitle(encodedTabTitle(instanceID, runID))
	}
	if runID != "" {
		_ = setWezTermUserVar("PHYTOZOME_GO_RUN_ID", runID)
	}
	if instanceID != "" {
		_ = setWezTermUserVar("PHYTOZOME_GO_INSTANCE_ID", instanceID)
	}
}

func windowTitle(version string) string {
	version = strings.TrimSpace(version)
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	return "PHgo (" + version + ")"
}

func setWezTermWindowTitle(title string) error {
	return runWezTermCLI("set-window-title", title)
}

func setWezTermTabTitle(title string) error {
	return runWezTermCLI("set-tab-title", title)
}

func runWezTermCLI(subcommand string, value string) error {
	subcommand = strings.TrimSpace(subcommand)
	value = strings.TrimSpace(value)
	if subcommand == "" || value == "" {
		return nil
	}
	if strings.TrimSpace(os.Getenv("WEZTERM_PANE")) == "" {
		return nil
	}
	launcher, err := weztermCLIPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(launcher, "cli", subcommand, value)
	cmd.Dir = filepath.Dir(launcher)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wezterm cli %s: %w", subcommand, err)
	}
	return nil
}

func setWezTermUserVar(name string, value string) error {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || value == "" {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	_, err := fmt.Fprintf(os.Stdout, "\x1b]1337;SetUserVar=%s=%s\x07", name, encoded)
	return err
}

func isRootInstanceID(instanceID string) bool {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return false
	}
	return !strings.Contains(instanceID, ".")
}

func encodedTabTitle(instanceID string, runID string) string {
	instanceID = strings.TrimSpace(instanceID)
	runID = strings.TrimSpace(runID)
	if instanceID == "" {
		return ""
	}
	return "__PHGO__|" + instanceID + "|" + runID
}
