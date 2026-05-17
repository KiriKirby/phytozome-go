package phygoboost

import (
	"context"
	"strings"
	"time"
)

type AsyncResult[T any] struct {
	Value T
	Err   error
}

func TimeoutContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func RunWithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	runCtx, cancel := TimeoutContext(ctx, timeout)
	defer cancel()
	return fn(runCtx)
}

func RunWithTimeoutValue[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if fn == nil {
		return zero, nil
	}
	runCtx, cancel := TimeoutContext(ctx, timeout)
	defer cancel()
	return fn(runCtx)
}

func RunTaskSpec(ctx context.Context, spec TaskSpec, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return nil
	}
	handle, runCtx, err := BindTaskSpec(ctx, spec)
	if err != nil {
		return err
	}
	if handle != nil {
		defer handle.Release()
	}
	return ObserveTaskSpec(runCtx, spec, fn)
}

func RunTaskSpecValue[T any](ctx context.Context, spec TaskSpec, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if fn == nil {
		return zero, nil
	}
	var out T
	err := RunTaskSpec(ctx, spec, func(runCtx context.Context) error {
		value, err := fn(runCtx)
		if err != nil {
			return err
		}
		out = value
		return nil
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}

func RunDisk(ctx context.Context, fn func(context.Context) error) error {
	return RunTaskSpec(ctx, TaskSpec{Level: ExecManaged, Description: "disk task"}, fn)
}

func RunDiskValue[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if fn == nil {
		return zero, nil
	}
	var out T
	err := RunDisk(ctx, func(runCtx context.Context) error {
		value, err := fn(runCtx)
		if err != nil {
			return err
		}
		out = value
		return nil
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}

func GoTask(ctx context.Context, spec TaskSpec, fn func(context.Context)) {
	if fn == nil {
		return
	}
	go func() {
		runCtx := ctx
		if runCtx == nil {
			runCtx = context.Background()
		}
		_ = RunTaskSpec(runCtx, spec, func(runCtx context.Context) error {
			fn(runCtx)
			return nil
		})
	}()
}

func StartAsyncResult[T any](ctx context.Context, fn func(context.Context) (T, error)) <-chan AsyncResult[T] {
	done := make(chan AsyncResult[T], 1)
	if fn == nil {
		close(done)
		return done
	}
	go func() {
		runCtx := ctx
		if runCtx == nil {
			runCtx = context.Background()
		}
		value, err := fn(runCtx)
		done <- AsyncResult[T]{Value: value, Err: err}
	}()
	return done
}

func MergeContext(parent context.Context, other context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if other == nil {
		return parent
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
		select {
		case <-parent.Done():
		case <-other.Done():
		case <-ctx.Done():
		}
	}()
	return ctx
}

func SleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func BindTaskSpec(ctx context.Context, spec TaskSpec) (*ResourceHandle, context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := missingResourceRequestForTaskSpec(ctx, spec)
	if resourceRequestIsEmpty(request) {
		return nil, ctx, nil
	}
	handle, err := DeclareResources(ctx, request)
	if err != nil {
		return nil, ctx, err
	}
	return handle, BindDeclaredResources(ctx, handle), nil
}

func workKindForTaskSpec(spec TaskSpec) workKind {
	if spec.Domain != "" || len(spec.Network) > 0 {
		return workNetwork
	}
	return workCPU
}

func resourceRequestForTaskSpec(spec TaskSpec) ResourceRequest {
	request := ResourceRequest{
		ManagedLevel: spec.Level,
		Description:  spec.Description,
	}
	if len(spec.Network) > 0 {
		request.Network = cloneNetworkBudget(spec.Network)
	}
	if spec.Domain != "" {
		if request.Network == nil {
			request.Network = make(map[string]int, 1)
		}
		if request.Network[spec.Domain] <= 0 {
			request.Network[spec.Domain] = 1
		}
	}
	if spec.Level != ExecUnmanaged {
		request.ManagedSlots = 1
	}
	return request
}

func cloneNetworkBudget(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for domain, slots := range values {
		domain = strings.TrimSpace(domain)
		if domain == "" || slots <= 0 {
			continue
		}
		out[domain] = slots
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func missingResourceRequestForTaskSpec(ctx context.Context, spec TaskSpec) ResourceRequest {
	request := resourceRequestForTaskSpec(spec)
	if spec.ForceOwnResources {
		return request
	}
	if request.ManagedSlots > 0 {
		if grant, ok := contextManagedGrant(ctx); ok && grant != nil {
			if request.ManagedLevel == grant.Level && grant.Slots >= request.ManagedSlots {
				request.ManagedSlots = 0
			}
		}
	}
	if len(request.Network) > 0 {
		for domain, slots := range request.Network {
			if slots <= 0 || contextHasNetworkGrant(ctx, domain) {
				delete(request.Network, domain)
			}
		}
		if len(request.Network) == 0 {
			request.Network = nil
		}
	}
	return request
}

func resourceRequestIsEmpty(request ResourceRequest) bool {
	return request.ManagedSlots <= 0 && len(request.Network) == 0
}
