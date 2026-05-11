package perf

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestParallelForRunsAllItems(t *testing.T) {
	var count atomic.Int64
	err := ParallelFor(context.Background(), WorkCPU, 25, func(ctx context.Context, index int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		count.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("ParallelFor returned error: %v", err)
	}
	if count.Load() != 25 {
		t.Fatalf("processed %d items, want 25", count.Load())
	}
}

func TestParallelForCancelsOnFirstError(t *testing.T) {
	expected := errors.New("boom")
	var seen atomic.Int64
	err := ParallelFor(context.Background(), WorkNetwork, 1000, func(ctx context.Context, index int) error {
		seen.Add(1)
		if index == 3 {
			return expected
		}
		return ctx.Err()
	})
	if !errors.Is(err, expected) {
		t.Fatalf("ParallelFor error = %v, want %v", err, expected)
	}
	if seen.Load() >= 1000 {
		t.Fatalf("ParallelFor did not stop early after error")
	}
}
