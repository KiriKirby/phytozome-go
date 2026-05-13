// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package netconfig

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultNetworkWorkersUsesAggressiveFloorAndEnvRaise(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "")
	base := DefaultNetworkWorkers()
	if base < 96 {
		t.Fatalf("DefaultNetworkWorkers()=%d, want at least 96", base)
	}

	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "4096")
	if got := DefaultNetworkWorkers(); got != 4096 {
		t.Fatalf("DefaultNetworkWorkers() with env=%d, want 4096", got)
	}
}

func TestNetworkWorkerCountBoundsByTotal(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "4096")
	if got := NetworkWorkerCount(17); got != 17 {
		t.Fatalf("NetworkWorkerCount(17)=%d, want 17", got)
	}
	if got := NetworkWorkerCount(0); got != 0 {
		t.Fatalf("NetworkWorkerCount(0)=%d, want 0", got)
	}
}

func TestDefaultHTTPClientUsesSharedPoolEnv(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "300")
	t.Setenv("PHYTOZOME_GO_MAX_IDLE_CONNS", "")
	t.Setenv("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", "")
	t.Setenv("PHYTOZOME_GO_HTTP_IDLE_SECONDS", "7")

	client := DefaultHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("DefaultHTTPClient transport type=%T, want *http.Transport", client.Transport)
	}
	if transport.MaxIdleConns != 600 {
		t.Fatalf("MaxIdleConns=%d, want 600", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 300 {
		t.Fatalf("MaxIdleConnsPerHost=%d, want 300", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 7*time.Second {
		t.Fatalf("IdleConnTimeout=%s, want 7s", transport.IdleConnTimeout)
	}
}
