// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"net"
	"net/http"
	"runtime"
	"time"
)

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          512,
			MaxIdleConnsPerHost:   128,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

func networkWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}
	workers := currentCPUCount() * 16
	if workers < 96 {
		workers = 96
	}
	if total < workers {
		return total
	}
	return workers
}

func defaultLocalBlastThreads() int {
	threads := currentCPUCount()
	if threads < 1 {
		return 1
	}
	if threads > runtime.NumCPU() {
		return runtime.NumCPU()
	}
	return threads
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
