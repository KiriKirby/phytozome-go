package phygoboost

import (
	"math"
	"net/http"
	"runtime"
	"time"
)

func Current() PoolControlProfile {
	state := RuntimeState()
	cpu := activeCPUCount()
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	snapshot := state.snapshot()
	uiReserve := uiReservedCPU(cpu, snapshot)
	managedTotal := managedTotalCapacity(cpu, uiReserve, snapshot)
	soft, hard := memoryLimits()
	return PoolControlProfile{
		CPU:               cpu,
		ManagedTotal:      managedTotal,
		UIReservedCPU:     uiReserve,
		MemorySoftLimit:   soft,
		MemoryHardLimit:   hard,
		SystemCPUPercent:  snapshot.SystemCPU,
		MemoryUsedPercent: memoryUsedPercent(snapshot),
		MaxIdleConns:      configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS", adaptiveHTTPIdleConnections(managedTotal)),
		MaxIdlePerHost:    configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", adaptiveHTTPIdlePerHost(managedTotal)),
		HTTPTimeout:       configuredDurationSeconds("PHYTOZOME_GO_HTTP_TIMEOUT_SECONDS", 60*time.Second),
		IdleConnTimeout:   configuredDurationSeconds("PHYTOZOME_GO_HTTP_IDLE_SECONDS", 90*time.Second),
		TLSHandshake:      configuredDurationSeconds("PHYTOZOME_GO_HTTP_TLS_SECONDS", 10*time.Second),
		ExpectContinue:    configuredDurationSeconds("PHYTOZOME_GO_HTTP_EXPECT_SECONDS", time.Second),
		MemoryCache:       configuredInt64("PHYTOZOME_GO_MEMORY_CACHE_BYTES", adaptiveMemoryCacheBudget(managedTotal, snapshot)),
	}
}

func HTTPClient() *http.Client {
	return HTTPClientForDomain("")
}

func MemoryCacheBudgetBytes() int64 {
	return Current().MemoryCache
}

func Budgets() BudgetProfile {
	profile := Current()
	return BudgetProfile{
		ManagedTotal:       profile.ManagedTotal,
		SubprocessParallel: profile.ManagedTotal,
	}
}

func UIThrottle() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_UI_THROTTLE_MS", 80*time.Millisecond)
}

func UIAnimationTick() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_UI_ANIMATION_MS", 120*time.Millisecond)
}

func SearchDebounce() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_SEARCH_DEBOUNCE_MS", 35*time.Millisecond)
}

func clampWorkers(total int, limit int) int {
	if total <= 0 || limit <= 0 {
		return 0
	}
	if total < limit {
		return total
	}
	return limit
}

func uiReservedCPU(cpu int, snapshot runtimeSnapshot) int {
	if cpu <= 2 {
		return 0
	}
	if env := configuredInt("PHYTOZOME_GO_UI_RESERVED_CPU", 0); env > 0 {
		if env >= cpu {
			return cpu - 1
		}
		return env
	}
	idle := cpuIdleRatio(snapshot)
	switch {
	case idle < 0.15:
		return minInt(cpu-1, 2)
	case idle < 0.35:
		return 1
	default:
		return 0
	}
}

func managedTotalCapacity(cpu int, uiReserve int, snapshot runtimeSnapshot) int {
	if cpu < 1 {
		cpu = 1
	}
	if uiReserve < 0 {
		uiReserve = 0
	}
	base := cpu - uiReserve
	if base < 1 {
		base = 1
	}
	if env := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); env > 0 {
		base = minPositive(base, env)
	}
	switch snapshot.Pressure {
	case PressureCritical:
		base = maxInt(1, base/4)
	case PressureHigh:
		base = maxInt(1, base/2)
	case PressureModerate:
		base = maxInt(1, int(math.Ceil(float64(base)*0.75)))
	}
	if snapshot.SystemCPU >= 95 {
		base = maxInt(1, base/2)
	} else if snapshot.SystemCPU >= 88 {
		base = maxInt(1, int(math.Ceil(float64(base)*0.7)))
	}
	return base
}

func cpuIdleRatio(snapshot runtimeSnapshot) float64 {
	if snapshot.SystemCPU <= 0 {
		return 0.55
	}
	return clampRatio((100 - snapshot.SystemCPU) / 100)
}

func memoryUsedPercent(snapshot runtimeSnapshot) float64 {
	if snapshot.SystemTotal == 0 {
		return 0
	}
	return clampRatio(float64(snapshot.SystemUsed)/float64(snapshot.SystemTotal)) * 100
}

func adaptiveHTTPIdleConnections(managedTotal int) int {
	return maxInt(16, managedTotal*4)
}

func adaptiveHTTPIdlePerHost(managedTotal int) int {
	return maxInt(4, managedTotal)
}

func adaptiveMemoryCacheBudget(managedTotal int, snapshot runtimeSnapshot) int64 {
	if snapshot.SystemTotal == 0 {
		return int64(maxInt(1, managedTotal)) * int64(pageSize()) * 2048
	}
	free := snapshot.SystemTotal - snapshot.SystemUsed
	share := float64(free) * 0.05
	perWorker := float64(maxInt(1, managedTotal)) * float64(pageSize()) * 256
	return int64(math.Max(share, perWorker))
}

func positiveCeil(value float64) int {
	if value <= 1 {
		return 1
	}
	if value > float64(int(^uint(0)>>1)) {
		return int(^uint(0) >> 1)
	}
	return int(math.Ceil(value))
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minPositive(a int, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func maxUint64(a uint64, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
