package phygoboost

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	pond "github.com/alitto/pond/v2"
)

type workKind string

const (
	workCPU     workKind = "cpu"
	workDisk    workKind = "disk"
	workNetwork workKind = "network"
)

type ParallelSpec struct {
	Level       ExecLevel
	Domain      string
	ForceOwnResources bool
	Description string
}

func ParallelForSpec(ctx context.Context, spec ParallelSpec, total int, fn func(context.Context, int) error) error {
	if total <= 0 || fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workers := clampWorkers(total, total)
	if workers <= 0 {
		workers = 1
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskCtx := context.WithValue(workCtx, parallelSpecContextKey{}, spec)
	pool := pond.NewPool(workers, pond.WithQueueSize(maxInt(workers, total)))
	defer pool.StopAndWait()

	var firstErr error
	var firstErrMu sync.Mutex
	rememberErr := func(err error) {
		if err == nil {
			return
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			firstErrMu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			firstErrMu.Unlock()
		}
		cancel()
	}

	tasks := make([]pond.Task, 0, total)
	for i := 0; i < total; i++ {
		index := i
		submittedAt := time.Now()
		task := pool.SubmitErr(func() error {
			request := requestForSpec(spec)
			if !spec.ForceOwnResources {
				if grant, ok := contextManagedGrant(taskCtx); ok && grant != nil {
					if request.ManagedSlots > 0 && request.ManagedLevel == grant.Level && grant.Slots >= request.ManagedSlots {
						request.ManagedSlots = 0
					}
				}
				if len(request.Network) > 0 {
					for domain, slots := range request.Network {
						if slots <= 0 || contextHasNetworkGrant(taskCtx, domain) {
							delete(request.Network, domain)
						}
					}
					if len(request.Network) == 0 {
						request.Network = nil
					}
				}
			}
			if resourceRequestIsEmpty(request) {
				startedAt := time.Now()
				err := fn(taskCtx, index)
				ObserveWork(workKindForParallelSpec(spec), time.Since(startedAt), startedAt.Sub(submittedAt), err)
				rememberErr(err)
				return err
			}
			handle, err := DeclareResources(taskCtx, request)
			if err != nil {
				ObserveWork(workKindForParallelSpec(spec), 0, time.Since(submittedAt), err)
				rememberErr(err)
				return err
			}
			defer handle.Release()
			runCtx := BindDeclaredResources(taskCtx, handle)
			if err := runCtx.Err(); err != nil {
				ObserveWork(workKindForParallelSpec(spec), 0, time.Since(submittedAt), err)
				rememberErr(err)
				return err
			}
			startedAt := time.Now()
			err = fn(runCtx, index)
			ObserveWork(workKindForParallelSpec(spec), time.Since(startedAt), startedAt.Sub(submittedAt), err)
			rememberErr(err)
			return err
		})
		tasks = append(tasks, task)
	}
	for len(tasks) > 0 {
		idx := waitParallelTask(tasks)
		task := tasks[idx]
		tasks = append(tasks[:idx], tasks[idx+1:]...)
		rememberErr(task.Wait())
	}
	firstErrMu.Lock()
	taskErr := firstErr
	firstErrMu.Unlock()
	if taskErr != nil {
		return taskErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

type parallelSpecContextKey struct{}

func waitParallelTask(tasks []pond.Task) int {
	if len(tasks) == 1 {
		<-tasks[0].Done()
		return 0
	}
	cases := make([]reflect.SelectCase, len(tasks))
	for i, task := range tasks {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.Done())}
	}
	chosen, _, _ := reflect.Select(cases)
	return chosen
}

func requestForSpec(spec ParallelSpec) ResourceRequest {
	request := ResourceRequest{
		ManagedLevel: spec.Level,
		Description:  spec.Description,
	}
	if spec.Level != ExecUnmanaged {
		request.ManagedSlots = 1
	}
	if spec.Domain != "" {
		request.Network = map[string]int{spec.Domain: 1}
		return request
	}
	return request
}

func workKindForParallelSpec(spec ParallelSpec) workKind {
	if spec.Domain != "" {
		return workNetwork
	}
	return workCPU
}

func closeParallelPools() {}
