package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func readClipboardText() (string, error) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "windows":
		script := "try { Get-Clipboard -Raw -ErrorAction Stop } catch { Add-Type -AssemblyName System.Windows.Forms; [Windows.Forms.Clipboard]::GetText() }"
		if path, err := exec.LookPath("pwsh"); err == nil {
			name = path
		} else {
			name = "powershell"
		}
		args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}
	case "darwin":
		name = "pbpaste"
	default:
		if _, err := exec.LookPath("wl-paste"); err == nil {
			name = "wl-paste"
			args = []string{"--no-newline"}
		} else if _, err := exec.LookPath("xclip"); err == nil {
			name = "xclip"
			args = []string{"-selection", "clipboard", "-out"}
		} else if _, err := exec.LookPath("xsel"); err == nil {
			name = "xsel"
			args = []string{"--clipboard", "--output"}
		} else {
			return "", fmt.Errorf("no supported clipboard command found")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := phygoboost.RunProcess(ctx, phygoboost.ProcessSpec{
		Name:   name,
		Args:   args,
		Task:   uiTaskSpec("read clipboard"),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("read clipboard: %w", ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("read clipboard: %w: %s", err, msg)
		}
		return "", fmt.Errorf("read clipboard: %w", err)
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

func writeClipboardText(text string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "windows":
		script := "Set-Clipboard -Value ([Console]::In.ReadToEnd())"
		if path, err := exec.LookPath("pwsh"); err == nil {
			name = path
		} else {
			name = "powershell"
		}
		args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}
	case "darwin":
		name = "pbcopy"
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			name = "wl-copy"
		} else if _, err := exec.LookPath("xclip"); err == nil {
			name = "xclip"
			args = []string{"-selection", "clipboard", "-in"}
		} else if _, err := exec.LookPath("xsel"); err == nil {
			name = "xsel"
			args = []string{"--clipboard", "--input"}
		} else {
			return fmt.Errorf("no supported clipboard command found")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	if err := phygoboost.RunProcess(ctx, phygoboost.ProcessSpec{
		Name:   name,
		Args:   args,
		Stdin:  strings.NewReader(text),
		Task:   uiTaskSpec("write clipboard"),
		Stdout: &bytes.Buffer{},
		Stderr: &stderr,
	}); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("write clipboard: %w", ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("write clipboard: %w: %s", err, msg)
		}
		return fmt.Errorf("write clipboard: %w", err)
	}
	return nil
}
