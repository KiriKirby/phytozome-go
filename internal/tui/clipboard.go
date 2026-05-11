package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func readClipboardText() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := "try { Get-Clipboard -Raw -ErrorAction Stop } catch { Add-Type -AssemblyName System.Windows.Forms; [Windows.Forms.Clipboard]::GetText() }"
		if path, err := exec.LookPath("pwsh"); err == nil {
			cmd = exec.Command(path, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
		} else {
			cmd = exec.Command("powershell", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
		}
	case "darwin":
		cmd = exec.Command("pbpaste")
	default:
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "--no-newline")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-out")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no supported clipboard command found")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cmd.Path != "" {
		cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
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
	return strings.TrimRight(string(out), "\r\n"), nil
}

func writeClipboardText(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := "Set-Clipboard -Value ([Console]::In.ReadToEnd())"
		if path, err := exec.LookPath("pwsh"); err == nil {
			cmd = exec.Command(path, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
		} else {
			cmd = exec.Command("powershell", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
		}
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-in")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no supported clipboard command found")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cmd.Path != "" {
		cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	}
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
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
