//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var version = "dev"

const displayName = "phytozome GO"

func main() {
	exePath, err := os.Executable()
	if err != nil {
		showError("Could not locate launcher executable: " + err.Error())
		os.Exit(1)
	}
	bundleDir := filepath.Dir(exePath)
	terminalPath := filepath.Join(bundleDir, "wezterm.bin")
	appPath := filepath.Join(bundleDir, "phytozome-go.bin")
	configPath := filepath.Join(bundleDir, "wezterm.lua")

	if err := requireFile(terminalPath, "GPU terminal runtime"); err != nil {
		showError(err.Error())
		os.Exit(1)
	}
	if err := requireFile(appPath, "phytozome GO runtime"); err != nil {
		showError(err.Error())
		os.Exit(1)
	}
	if err := requireFile(configPath, "WezTerm configuration"); err != nil {
		showError(err.Error())
		os.Exit(1)
	}

	runtimeDir, err := prepareWezTermRuntime(bundleDir)
	if err != nil {
		showError(err.Error())
		os.Exit(1)
	}
	runtimeTerminalPath := filepath.Join(runtimeDir, "wezterm-gui.exe")
	args := []string{
		"--config-file", configPath,
		"start",
		"--always-new-process",
		"--cwd", bundleDir,
		"--",
		appPath,
	}
	args = append(args, os.Args[1:]...)

	cmd := exec.Command(runtimeTerminalPath, args...)
	cmd.Dir = bundleDir
	cmd.Env = append(os.Environ(), "WEZTERM_CONFIG_FILE="+configPath)
	if err := cmd.Start(); err != nil {
		showError("Could not start bundled WezTerm terminal:\n\n" + err.Error())
		os.Exit(1)
	}
}

func prepareWezTermRuntime(bundleDir string) (string, error) {
	runtimeDir := filepath.Join(os.TempDir(), "phytozome-go-wezterm-runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", fmt.Errorf("create WezTerm runtime directory:\n%w", err)
	}
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return "", fmt.Errorf("read bundle directory:\n%w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if strings.EqualFold(entry.Name(), "mesa") {
				if err := copyDir(filepath.Join(bundleDir, entry.Name()), filepath.Join(runtimeDir, entry.Name())); err != nil {
					return "", err
				}
			}
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		source := filepath.Join(bundleDir, name)
		targetName := name
		switch lower {
		case "wezterm.bin":
			targetName = "wezterm-gui.exe"
		case "wezterm-mux-server.bin":
			targetName = "wezterm-mux-server.exe"
		case "openconsole.bin":
			targetName = "OpenConsole.exe"
		default:
			if !strings.HasSuffix(lower, ".dll") {
				continue
			}
		}
		if err := copyFileIfNeeded(source, filepath.Join(runtimeDir, targetName)); err != nil {
			return "", err
		}
	}
	if err := requireFile(filepath.Join(runtimeDir, "wezterm-gui.exe"), "prepared WezTerm GUI runtime"); err != nil {
		return "", err
	}
	return runtimeDir, nil
}

func copyDir(sourceDir string, targetDir string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("read runtime source directory:\n%s\n%w", sourceDir, err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create runtime target directory:\n%s\n%w", targetDir, err)
	}
	for _, entry := range entries {
		source := filepath.Join(sourceDir, entry.Name())
		target := filepath.Join(targetDir, entry.Name())
		if entry.IsDir() {
			if err := copyDir(source, target); err != nil {
				return err
			}
			continue
		}
		if err := copyFileIfNeeded(source, target); err != nil {
			return err
		}
	}
	return nil
}

func copyFileIfNeeded(source string, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("read runtime source file:\n%s\n%w", source, err)
	}
	if targetInfo, err := os.Stat(target); err == nil && targetInfo.Size() == sourceInfo.Size() && targetInfo.ModTime().Equal(sourceInfo.ModTime()) {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open runtime source file:\n%s\n%w", source, err)
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create runtime target file:\n%s\n%w", target, err)
	}
	defer output.Close()
	if _, err := output.ReadFrom(input); err != nil {
		return fmt.Errorf("copy runtime file:\n%s\n%w", source, err)
	}
	_ = os.Chtimes(target, sourceInfo.ModTime(), sourceInfo.ModTime())
	return nil
}

func requireFile(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("missing %s:\n%s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, expected a file:\n%s", label, path)
	}
	return nil
}

func showError(message string) {
	title, _ := syscall.UTF16PtrFromString(displayName)
	text, _ := syscall.UTF16PtrFromString(message)
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	const (
		mbOK        = 0x00000000
		mbIconStop  = 0x00000010
		mbSetFocus  = 0x00010000
		mbTopmost   = 0x00040000
		messageFlag = mbOK | mbIconStop | mbSetFocus | mbTopmost
	)
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), messageFlag)
}
