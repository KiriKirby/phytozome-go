package perf

import (
	"testing"
	"time"
)

func TestCurrentProfileHonorsEnvironmentOverrides(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "321")
	t.Setenv("PHYTOZOME_GO_DISK_WORKERS", "11")
	t.Setenv("PHYTOZOME_GO_UI_THROTTLE_MS", "17")
	t.Setenv("PHYTOZOME_GO_SEARCH_DEBOUNCE_MS", "23")
	t.Setenv("PHYTOZOME_GO_MAX_IDLE_CONNS", "777")
	t.Setenv("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", "333")

	profile := Current()
	if profile.NetworkWorkers != 321 {
		t.Fatalf("NetworkWorkers = %d, want 321", profile.NetworkWorkers)
	}
	if profile.DiskWorkers != 11 {
		t.Fatalf("DiskWorkers = %d, want 11", profile.DiskWorkers)
	}
	if profile.UIThrottle != 17*time.Millisecond {
		t.Fatalf("UIThrottle = %v, want 17ms", profile.UIThrottle)
	}
	if profile.SearchDebounce != 23*time.Millisecond {
		t.Fatalf("SearchDebounce = %v, want 23ms", profile.SearchDebounce)
	}
	if profile.MaxIdleConns != 777 {
		t.Fatalf("MaxIdleConns = %d, want 777", profile.MaxIdleConns)
	}
	if profile.MaxIdlePerHost != 333 {
		t.Fatalf("MaxIdlePerHost = %d, want 333", profile.MaxIdlePerHost)
	}
}

func TestWorkerKindsClampToTotal(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "999")
	if got := NetworkWorkers(5); got != 5 {
		t.Fatalf("NetworkWorkers(5) = %d, want 5", got)
	}
	if got := DiskWorkers(3); got != 3 {
		t.Fatalf("DiskWorkers(3) = %d, want 3", got)
	}
	if got := CPUWorkers(2); got != 2 {
		t.Fatalf("CPUWorkers(2) = %d, want 2", got)
	}
}
