// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func defaultHTTPClient() *http.Client {
	cpu := currentCPUCount()
	networkWorkers := defaultNetworkWorkers()
	maxIdleConns := configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS", maxInt(networkWorkers*2, 512))
	maxIdleConnsPerHost := configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", maxInt(networkWorkers, 128))
	httpTimeout := configuredDurationSeconds("PHYTOZOME_GO_HTTP_TIMEOUT_SECONDS", 60*time.Second)
	idleConnTimeout := configuredDurationSeconds("PHYTOZOME_GO_HTTP_IDLE_SECONDS", 90*time.Second)
	tlsHandshakeTimeout := configuredDurationSeconds("PHYTOZOME_GO_HTTP_TLS_SECONDS", 10*time.Second)
	expectContinueTimeout := configuredDurationSeconds("PHYTOZOME_GO_HTTP_EXPECT_SECONDS", time.Second)

	_ = cpu
	return &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: tlsHandshakeTimeout, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		},
	}
}

func currentCPUCount() int {
	cpu := runtime.GOMAXPROCS(0)
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	if cpu < 1 {
		return 1
	}
	return cpu
}

func defaultNetworkWorkers() int {
	cpu := currentCPUCount()
	workers := maxInt(cpu*16, 96)
	if envWorkers := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envWorkers > workers {
		workers = envWorkers
	}
	return workers
}

func defaultDiskWorkers() int {
	cpu := currentCPUCount()
	workers := maxInt(2, minInt(cpu, 8))
	if envWorkers := configuredInt("PHYTOZOME_GO_DISK_WORKERS", 0); envWorkers > 0 {
		workers = envWorkers
	}
	return workers
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
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func configuredDurationSeconds(name string, fallback time.Duration) time.Duration {
	value := configuredInt(name, 0)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}
