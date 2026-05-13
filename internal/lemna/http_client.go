// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"net/http"
	"runtime"

	"github.com/KiriKirby/phytozome-go/internal/netconfig"
)

func defaultHTTPClient() *http.Client {
	return netconfig.DefaultHTTPClient()
}

func networkWorkerCount(total int) int {
	return netconfig.NetworkWorkerCount(total)
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
	return netconfig.CurrentCPUCount()
}
