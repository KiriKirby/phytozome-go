package phygoboost

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func TestSingleflightDoContextReturnsOnCallerCancellation(t *testing.T) {
	var group singleflight.Group
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	go func() {
		_, _, _ = SingleflightDoContext(context.Background(), &group, "species", func() (any, error) {
			calls.Add(1)
			close(started)
			<-release
			return "ok", nil
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared call did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err, _ := SingleflightDoContext(ctx, &group, "species", func() (any, error) {
			t.Fatal("cancelled waiter should not execute callback")
			return nil, nil
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("cancelled waiter stayed blocked on shared call")
	}

	close(release)

	if got := calls.Load(); got != 1 {
		t.Fatalf("callback count = %d, want 1", got)
	}
}
