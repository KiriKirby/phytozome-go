package tui

import (
	"context"
	"errors"
)

var errTUIWorkerUnavailable = errors.New("tui worker unavailable")

func runPageInWorkerIfNeeded[P any, R any](taskName string, page P, result *R) (bool, error) {
	return false, nil
}

func runTaskInUIWorker[T any](ctx context.Context, page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	var zero T
	return zero, errTUIWorkerUnavailable
}

func runTaskInUIWorkerWithProgress[T any](ctx context.Context, page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	var zero T
	return zero, errTUIWorkerUnavailable
}

func inTUIWorker() bool {
	return false
}

func tuiWorkerEnabled() bool {
	return false
}

func tuiPageWorkerEnabled() bool {
	return false
}

func tuiTaskWorkerEnabled() bool {
	return false
}
