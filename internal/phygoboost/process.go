package phygoboost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type ProcessSpec struct {
	Name   string
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Task   TaskSpec
	Stdout io.Writer
	Stderr io.Writer
}

func RunProcess(ctx context.Context, spec ProcessSpec) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("process name is empty")
	}
	task := spec.Task
	if task.Level == ExecUnmanaged && task.Domain == "" && len(task.Network) == 0 {
		return fmt.Errorf("process task spec is required for %s", spec.Name)
	}
	taskKind := workKindForTaskSpec(task)
	request := missingResourceRequestForTaskSpec(ctx, task)
	var (
		handle *ResourceHandle
		err    error
		runCtx = ctx
	)
	if !resourceRequestIsEmpty(request) {
		handle, err = DeclareResources(ctx, request)
		if err != nil {
			return err
		}
		defer handle.Release()
		runCtx = BindDeclaredResources(ctx, handle)
	}
	done := WorkerStarted()
	defer done()
	started := time.Now()
	err = func(runCtx context.Context) error {
		cmd := exec.CommandContext(runCtx, spec.Name, spec.Args...)
		if spec.Dir != "" {
			cmd.Dir = spec.Dir
		}
		if spec.Stdin != nil {
			cmd.Stdin = spec.Stdin
		}
		if len(spec.Env) > 0 {
			cmd.Env = append(cmd.Environ(), spec.Env...)
		}
		var stderr bytes.Buffer
		if spec.Stdout != nil {
			cmd.Stdout = spec.Stdout
		} else {
			cmd.Stdout = io.Discard
		}
		if spec.Stderr != nil {
			cmd.Stderr = spec.Stderr
		} else {
			cmd.Stderr = &stderr
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w%s", spec.Name, err, FormatCapturedOutput(stderr.String()))
		}
		return nil
	}(runCtx)
	ObserveWork(taskKind, time.Since(started), 0, err)
	return err
}

func FormatCapturedOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 8 {
		lines = append(lines[:8], "...")
	}
	return "\n" + strings.Join(lines, "\n")
}
