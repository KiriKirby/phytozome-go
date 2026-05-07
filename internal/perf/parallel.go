package perf

import (
	"context"
	"sync"
)

type WorkKind string

const (
	WorkCPU     WorkKind = "cpu"
	WorkDisk    WorkKind = "disk"
	WorkNetwork WorkKind = "network"
)

func Workers(kind WorkKind, total int) int {
	switch kind {
	case WorkCPU:
		return CPUWorkers(total)
	case WorkDisk:
		return DiskWorkers(total)
	case WorkNetwork:
		return NetworkWorkers(total)
	default:
		return DynamicWorkers(total, 0)
	}
}

func ParallelFor(ctx context.Context, kind WorkKind, total int, fn func(context.Context, int) error) error {
	if total <= 0 || fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workerCount := Workers(kind, total)
	if workerCount <= 0 {
		return nil
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	errs := make(chan error, 1)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if err := workCtx.Err(); err != nil {
					return
				}
				if err := fn(workCtx, index); err != nil {
					select {
					case errs <- err:
						cancel()
					default:
					}
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range total {
			select {
			case <-workCtx.Done():
				return
			case jobs <- i:
			}
		}
	}()
	workers.Wait()
	select {
	case err := <-errs:
		return err
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
