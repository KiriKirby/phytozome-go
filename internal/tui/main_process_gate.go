package tui

import (
	"context"
	"errors"
)

var errTUIMustRunInMainProcess = errors.New("tui task must run in main process")

func runPageOutsideMainProcessIfNeeded[P any, R any](taskName string, page P, result *R) (bool, error) {
	return false, nil
}

func runTaskOutsideMainProcess[T any](ctx context.Context, page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	var zero T
	return zero, errTUIMustRunInMainProcess
}

func runTaskOutsideMainProcessWithProgress[T any](ctx context.Context, page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	var zero T
	return zero, errTUIMustRunInMainProcess
}

func inAuxiliaryTUIProcess() bool {
	return false
}

func tuiParallelProcessEnabled() bool {
	return false
}

func tuiPageParallelProcessEnabled() bool {
	return false
}

func tuiTaskParallelProcessEnabled() bool {
	return false
}
