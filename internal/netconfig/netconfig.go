// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package netconfig

import (
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func DefaultHTTPClient() *http.Client {
	networkWorkers := DefaultNetworkWorkers()
	maxIdleConns := ConfiguredInt("PHYTOZOME_GO_MAX_IDLE_CONNS", maxInt(networkWorkers*2, 512))
	maxIdleConnsPerHost := ConfiguredInt("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", maxInt(networkWorkers, 128))
	idleConnTimeout := ConfiguredDurationSeconds("PHYTOZOME_GO_HTTP_IDLE_SECONDS", 90*time.Second)

	return &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     idleConnTimeout,
		},
	}
}

func CurrentCPUCount() int {
	cpu := runtime.GOMAXPROCS(0)
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	if cpu < 1 {
		return 1
	}
	return cpu
}

func DefaultNetworkWorkers() int {
	workers := maxInt(CurrentCPUCount()*16, 96)
	if envWorkers := ConfiguredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envWorkers > workers {
		workers = envWorkers
	}
	return workers
}

func DefaultDiskWorkers() int {
	workers := maxInt(2, minInt(CurrentCPUCount(), 8))
	if envWorkers := ConfiguredInt("PHYTOZOME_GO_DISK_WORKERS", 0); envWorkers > 0 {
		workers = envWorkers
	}
	return workers
}

func NetworkWorkerCount(total int) int {
	return BoundedWorkerCount(total, DefaultNetworkWorkers())
}

func BoundedWorkerCount(total int, limit int) int {
	if total <= 0 {
		return 0
	}
	if limit <= 0 {
		limit = 1
	}
	if total < limit {
		return total
	}
	return limit
}

func ConfiguredInt(name string, fallback int) int {
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

func ConfiguredDurationSeconds(name string, fallback time.Duration) time.Duration {
	value := ConfiguredInt(name, 0)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
