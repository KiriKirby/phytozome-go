// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/netconfig"
)

func defaultHTTPClient() *http.Client {
	return netconfig.DefaultHTTPClient()
}

func currentCPUCount() int {
	return netconfig.CurrentCPUCount()
}

func defaultNetworkWorkers() int {
	return netconfig.DefaultNetworkWorkers()
}

func defaultDiskWorkers() int {
	return netconfig.DefaultDiskWorkers()
}

func clampWorkers(total int, preferredMax int) int {
	if total <= 0 {
		return 0
	}
	limit := preferredMax
	if limit <= 0 {
		limit = defaultNetworkWorkers()
	}
	cpu := currentCPUCount()
	if limit < cpu {
		limit = cpu
	}
	if envLimit := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envLimit > limit {
		limit = envLimit
	}
	if total < limit {
		return total
	}
	return limit
}

func runParallel(ctx context.Context, total int, workerCount int, fn func(context.Context, int) error) error {
	if total <= 0 || fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workerCount <= 0 {
		workerCount = total
	}
	if workerCount > total {
		workerCount = total
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

func configuredInt(name string, fallback int) int {
	return netconfig.ConfiguredInt(name, fallback)
}

func configuredDurationSeconds(name string, fallback time.Duration) time.Duration {
	return netconfig.ConfiguredDurationSeconds(name, fallback)
}
