package phygoboost

import (
	"net/http"
	"math"
	"runtime"
	"time"
)

type Profile struct {
	CPU               int
	BackgroundCPU     int
	UIReservedCPU     int
	PageSize          int64
	NetworkWorkers    int
	DiskWorkers       int
	ProcessWorkers    int
	ActiveWorkers     int
	ChildProcesses    int
	WorkerMode        bool
	WorkerGOMAXPROCS  int
	WorkerMemoryLimit uint64
	MaxIdleConns      int
	MaxIdlePerHost    int
	UIThrottle        time.Duration
	UIAnimationTick   time.Duration
	SearchDebounce    time.Duration
	HTTPTimeout       time.Duration
	IdleConnTimeout   time.Duration
	TLSHandshake      time.Duration
	ExpectContinue    time.Duration
	RetryMax          int
	RateLimit         int
	MemorySoftLimit   uint64
	MemoryHardLimit   uint64
	MemoryCache       int64
	SystemCPUPercent  float64
	MemoryUsedPercent float64
	Feedback          map[string]WorkFeedback
}

type runtimeSnapshot struct {
	ProcessRSS     uint64
	SystemUsed     uint64
	SystemTotal    uint64
	SystemCPU      float64
	ChildProcesses int
	ActiveWorkers  int
	Pressure       PressureLevel
}

func Current() Profile {
	state := RuntimeState()
	cpu := activeCPUCount()
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	workerMode := workerProcessMode()
	snapshot := state.snapshot()
	uiReserve := uiReservedCPU(cpu, workerMode, snapshot)
	backgroundCPU := maxInt(1, cpu-uiReserve)
	maxWorkers := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0)
	networkWorkers := adaptiveWorkerBudget(workNetwork, cpu, backgroundCPU, workerMode, snapshot)
	if envNetwork := configuredInt("PHYTOZOME_GO_NETWORK_WORKERS", 0); envNetwork > 0 {
		networkWorkers = envNetwork
	} else if maxWorkers > 0 {
		networkWorkers = maxWorkers
	}
	networkWorkers = state.AdjustWorkers(workNetwork, networkWorkers)
	diskWorkers := adaptiveWorkerBudget(workDisk, cpu, backgroundCPU, workerMode, snapshot)
	if envDisk := configuredInt("PHYTOZOME_GO_DISK_WORKERS", 0); envDisk > 0 {
		diskWorkers = envDisk
	}
	diskWorkers = state.AdjustWorkers(workDisk, capByMaxWorkers(diskWorkers, maxWorkers))
	processWorkers := adaptiveWorkerBudget(workProcess, cpu, backgroundCPU, workerMode, snapshot)
	if envProcess := configuredInt("PHYTOZOME_GO_PROCESS_WORKERS", 0); envProcess > 0 {
		processWorkers = envProcess
	}
	processWorkers = state.AdjustWorkers(workProcess, capByMaxWorkers(processWorkers, maxWorkers))
	maxIdle := configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS", adaptiveHTTPIdleConnections(networkWorkers, snapshot))
	soft, hard := memoryLimits()
	activeWorkers := snapshot.ActiveWorkers
	childProcesses := snapshot.ChildProcesses
	feedback := state.feedbackSnapshot()
	return Profile{
		CPU:               cpu,
		BackgroundCPU:     backgroundCPU,
		UIReservedCPU:     uiReserve,
		PageSize:          pageSize(),
		NetworkWorkers:    networkWorkers,
		DiskWorkers:       diskWorkers,
		ProcessWorkers:    processWorkers,
		ActiveWorkers:     activeWorkers,
		ChildProcesses:    childProcesses,
		WorkerMode:        workerMode,
		WorkerGOMAXPROCS:  workerGOMAXPROCS(workProcess, backgroundCPU, processWorkers),
		WorkerMemoryLimit: workerMemoryLimitBytes(hard, processWorkers, activeWorkers, childProcesses),
		MaxIdleConns:      maxIdle,
		MaxIdlePerHost:    configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", adaptiveHTTPIdlePerHost(networkWorkers, snapshot)),
		UIThrottle:        configuredDurationMS("PHYTOZOME_GO_UI_THROTTLE_MS", 80*time.Millisecond),
		UIAnimationTick:   configuredDurationMS("PHYTOZOME_GO_UI_ANIMATION_MS", 120*time.Millisecond),
		SearchDebounce:    configuredDurationMS("PHYTOZOME_GO_SEARCH_DEBOUNCE_MS", 35*time.Millisecond),
		HTTPTimeout:       configuredDurationSeconds("PHYTOZOME_GO_HTTP_TIMEOUT_SECONDS", 60*time.Second),
		IdleConnTimeout:   configuredDurationSeconds("PHYTOZOME_GO_HTTP_IDLE_SECONDS", 90*time.Second),
		TLSHandshake:      configuredDurationSeconds("PHYTOZOME_GO_HTTP_TLS_SECONDS", 10*time.Second),
		ExpectContinue:    configuredDurationSeconds("PHYTOZOME_GO_HTTP_EXPECT_SECONDS", time.Second),
		RetryMax:          configuredInt("PHYTOZOME_GO_HTTP_RETRY_MAX", 3),
		RateLimit:         configuredInt("PHYTOZOME_GO_HTTP_RATE_LIMIT", adaptiveHTTPRateLimit(networkWorkers, snapshot)),
		MemorySoftLimit:   soft,
		MemoryHardLimit:   hard,
		MemoryCache:       configuredInt64("PHYTOZOME_GO_MEMORY_CACHE_BYTES", adaptiveMemoryCacheBudget(networkWorkers, snapshot)),
		SystemCPUPercent:  snapshot.SystemCPU,
		MemoryUsedPercent: memoryUsedPercent(snapshot),
		Feedback:          feedback,
	}
}

func HTTPClient() *http.Client {
	return HTTPClientForDomain("")
}

func DynamicWorkers(total int, preferredMax int) int {
	if total <= 0 {
		return 0
	}
	limit := preferredMax
	if limit <= 0 {
		limit = Current().NetworkWorkers
	}
	if envLimit := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envLimit > 0 {
		limit = minPositive(limit, RuntimeState().AdjustWorkers(workNetwork, envLimit))
	}
	return clampWorkers(total, limit)
}

func CPUWorkers(total int) int {
	profile := Current()
	limit := profile.BackgroundCPU
	if envLimit := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envLimit > 0 {
		limit = minPositive(limit, envLimit)
	}
	return clampWorkers(total, limit)
}

func DiskWorkers(total int) int {
	return clampWorkers(total, Current().DiskWorkers)
}

func NetworkWorkers(total int) int {
	return NetworkRequestWorkers(total)
}

func NetworkRequestWorkers(total int) int {
	if total <= 0 {
		return 0
	}
	profile := Current()
	return clampWorkers(total, adaptiveNetworkRequestBudget(profile))
}

func NetworkProcessWorkers(total int) int {
	if total <= 0 {
		return 0
	}
	profile := Current()
	return clampWorkers(total, adaptiveNetworkProcessBudget(profile))
}

func BackgroundPrefetchWorkers(total int) int {
	if total <= 0 {
		return 0
	}
	profile := Current()
	if profile.MemoryUsedPercent > 80 || profile.SystemCPUPercent > 86 {
		return 0
	}
	if profile.ActiveWorkers+profile.ChildProcesses > profile.BackgroundCPU {
		return 0
	}
	if feedback, ok := profile.Feedback[string(workNetwork)]; ok && feedback.Count > 0 {
		if feedback.Failures > 0 && float64(feedback.Failures)/float64(feedback.Count) > 0.18 {
			return 0
		}
		if feedback.Duration > 0 && feedback.QueueDelay > feedback.Duration*2 {
			return 0
		}
	}
	workers := positiveCeil(math.Sqrt(float64(adaptiveNetworkRequestBudget(profile))))
	if workers > profile.BackgroundCPU {
		workers = profile.BackgroundCPU
	}
	return clampWorkers(total, workers)
}

func ProcessWorkers(total int) int {
	return clampWorkers(total, Current().ProcessWorkers)
}

func MemoryCacheBudgetBytes() int64 {
	return Current().MemoryCache
}

func Budgets() BudgetProfile {
	profile := Current()
	return BudgetProfile{
		LocalMain:        maxInt(1, profile.BackgroundCPU),
		LocalHeavy:       maxInt(1, profile.ProcessWorkers),
		NetworkMain:      maxInt(1, adaptiveNetworkRequestBudget(profile)),
		PrefetchMain:     BackgroundPrefetchWorkers(maxInt(1, adaptiveNetworkRequestBudget(profile))),
		WorkerGOMAXPROCS: maxInt(1, profile.WorkerGOMAXPROCS),
	}
}

func UIThrottle() time.Duration {
	return Current().UIThrottle
}

func UIAnimationTick() time.Duration {
	return Current().UIAnimationTick
}

func SearchDebounce() time.Duration {
	return Current().SearchDebounce
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

func uiReservedCPU(cpu int, workerMode bool, snapshot runtimeSnapshot) int {
	if workerMode || cpu <= 2 {
		return 0
	}
	if env := configuredInt("PHYTOZOME_GO_UI_RESERVED_CPU", 0); env > 0 {
		if env >= cpu {
			return cpu - 1
		}
		return env
	}
	idle := cpuIdleRatio(snapshot)
	reserve := float64(cpu) * (1 - idle) * uiReserveRatio(snapshot)
	return minInt(cpu-1, positiveCeil(reserve))
}

func childWorkerNetworkWorkers(kind workKind, profile Profile) int {
	return childWorkerBudget(kind == workNetwork, profile.NetworkWorkers, profile)
}

func childWorkerDiskWorkers(kind workKind, profile Profile) int {
	return childWorkerBudget(kind == workDisk, profile.DiskWorkers, profile)
}

func childWorkerProcessWorkers(profile Profile) int {
	return childWorkerBudget(true, profile.ProcessWorkers, profile)
}

func childWorkerBudget(primary bool, parentBudget int, profile Profile) int {
	if parentBudget < 1 {
		return 1
	}
	live := maxInt(1, profile.ActiveWorkers+profile.ChildProcesses+1)
	share := float64(parentBudget) / float64(live)
	if !primary {
		share *= cpuIdleRatio(runtimeSnapshot{SystemCPU: profile.SystemCPUPercent})
	}
	return positiveCeil(share)
}

func externalNativeThreads(gomax int, profile Profile) int {
	live := maxInt(1, profile.ActiveWorkers+profile.ChildProcesses+1)
	return positiveCeil(float64(gomax) / math.Sqrt(float64(live)))
}

func adaptiveWorkerBudget(kind workKind, cpu int, backgroundCPU int, workerMode bool, snapshot runtimeSnapshot) int {
	if backgroundCPU < 1 {
		backgroundCPU = maxInt(1, cpu)
	}
	idle := cpuIdleRatio(snapshot)
	memFree := memoryFreeRatio(snapshot)
	contention := contentionScale(snapshot, backgroundCPU)
	pressure := pressureScale(snapshot.Pressure)
	scope := 1.0
	if workerMode {
		scope = childScopeScale(snapshot, backgroundCPU)
	}
	feedback := RuntimeState().feedback(kind)
	feedbackScale := feedbackBudgetScale(feedback)

	var raw float64
	switch kind {
	case workCPU:
		raw = float64(backgroundCPU) * (0.35 + idle) * (0.55 + memFree) * pressure * contention * scope * feedbackScale
	case workDisk:
		raw = math.Sqrt(float64(backgroundCPU)) * (0.5 + idle) * (0.5 + memFree) * pressure * contention * scope * feedbackScale
	case workProcess:
		raw = float64(backgroundCPU) * (0.3 + idle) * (0.45 + memFree) * pressure * contention * processContentionScale(snapshot, backgroundCPU) * scope * feedbackScale
	default:
		raw = float64(backgroundCPU) * (1.1 + idle*1.6 + memFree) * pressure * math.Sqrt(contention) * scope * feedbackScale
	}
	return positiveCeil(raw)
}

func feedbackBudgetScale(feedback workFeedback) float64 {
	if feedback.Count == 0 {
		return 1
	}
	scale := 1.0
	if feedback.DurationEWMA > 0 && feedback.QueueEWMA > 0 {
		ratio := feedback.QueueEWMA / feedback.DurationEWMA
		switch {
		case ratio > 1.0:
			scale *= 1.35
		case ratio > 0.45:
			scale *= 1.18
		case ratio < 0.05:
			scale *= 0.92
		}
	}
	if feedback.Failures > 0 {
		failureRate := float64(feedback.Failures) / float64(feedback.Count)
		scale *= 1 - clampRatio(failureRate)*0.65
	}
	if feedback.DurationEWMA > 0 && feedback.LastDuration > 0 {
		change := feedback.LastDuration / feedback.DurationEWMA
		switch {
		case change > 1.5:
			scale *= 0.82
		case change > 1.2:
			scale *= 0.92
		case change < 0.75 && feedback.QueueEWMA < feedback.DurationEWMA*0.25:
			scale *= 1.08
		}
	}
	if feedback.ThroughputEWMA > 0 && feedback.LastDuration > 0 {
		lastThroughput := 1 / (feedback.LastDuration / float64(time.Second))
		if lastThroughput < feedback.ThroughputEWMA*0.72 {
			scale *= 0.9
		} else if lastThroughput > feedback.ThroughputEWMA*1.18 && feedback.QueueEWMA > 0 {
			scale *= 1.06
		}
	}
	if scale < 0.25 {
		return 0.25
	}
	if scale > 1.75 {
		return 1.75
	}
	return scale
}

func adaptiveNetworkRequestBudget(profile Profile) int {
	base := profile.NetworkWorkers
	if base < 1 {
		base = 1
	}
	scale := networkExternalScale(profile)
	budget := positiveCeil(float64(base) * scale)
	if feedback, ok := profile.Feedback[string(workNetwork)]; ok && feedback.Count > 0 {
		if feedback.QueueDelay > feedback.Duration && feedback.Duration > 0 && feedback.Failures == 0 {
			budget = positiveCeil(float64(budget) * 1.15)
		}
		if feedback.Failures > 0 {
			failureRate := float64(feedback.Failures) / float64(feedback.Count)
			budget = positiveCeil(float64(budget) * (1 - clampRatio(failureRate)*0.7))
		}
	}
	if budget < 1 {
		budget = 1
	}
	return budget
}

func adaptiveNetworkProcessBudget(profile Profile) int {
	requestBudget := adaptiveNetworkRequestBudget(profile)
	if requestBudget < 1 {
		return 1
	}
	processes := positiveCeil(math.Sqrt(float64(requestBudget)))
	if profile.ChildProcesses > profile.BackgroundCPU {
		processes = positiveCeil(float64(processes) * 0.7)
	}
	if profile.MemoryUsedPercent > 82 {
		processes = positiveCeil(float64(processes) * 0.6)
	}
	if feedback, ok := profile.Feedback[string(workNetwork)]; ok && feedback.Failures > 0 && feedback.Count > 0 {
		failureRate := float64(feedback.Failures) / float64(feedback.Count)
		processes = positiveCeil(float64(processes) * (1 - clampRatio(failureRate)*0.65))
	}
	return maxInt(1, processes)
}

func networkExternalScale(profile Profile) float64 {
	scale := 1.0
	if profile.SystemCPUPercent > 0 {
		idle := clampRatio((100 - profile.SystemCPUPercent) / 100)
		scale *= 0.55 + idle*0.65
	}
	if profile.MemoryUsedPercent > 0 {
		free := clampRatio((100 - profile.MemoryUsedPercent) / 100)
		scale *= 0.45 + free*0.75
	}
	if profile.ActiveWorkers+profile.ChildProcesses > profile.BackgroundCPU {
		scale *= 0.72
	}
	if scale < 0.18 {
		return 0.18
	}
	if scale > 1.15 {
		return 1.15
	}
	return scale
}

func ewmaDuration(current float64, value time.Duration, alpha float64) float64 {
	next := float64(value)
	if current <= 0 {
		return next
	}
	return current*(1-alpha) + next*alpha
}

func ewmaFloat(current float64, next float64, alpha float64) float64 {
	if next <= 0 {
		return current
	}
	if current <= 0 {
		return next
	}
	return current*(1-alpha) + next*alpha
}

func runtimeScale(kind workKind, snapshot runtimeSnapshot) float64 {
	idle := cpuIdleRatio(snapshot)
	memFree := memoryFreeRatio(snapshot)
	scale := pressureScale(snapshot.Pressure) * (0.45 + idle) * (0.45 + memFree)
	if kind == workNetwork {
		scale *= 0.75 + memFree
	}
	if scale < 0.05 {
		return 0.05
	}
	return scale
}

func processContentionScale(snapshot runtimeSnapshot, budget int) float64 {
	live := snapshot.ActiveWorkers + snapshot.ChildProcesses
	if live <= 0 || budget <= 0 {
		return 1
	}
	return 1 / (1 + float64(live)/float64(budget))
}

func contentionScale(snapshot runtimeSnapshot, backgroundCPU int) float64 {
	live := snapshot.ActiveWorkers + snapshot.ChildProcesses
	if live <= 0 || backgroundCPU <= 0 {
		return 1
	}
	return 1 / (1 + float64(live)/float64(backgroundCPU))
}

func childScopeScale(snapshot runtimeSnapshot, backgroundCPU int) float64 {
	live := snapshot.ActiveWorkers + snapshot.ChildProcesses + 1
	if backgroundCPU <= 0 {
		return 1 / float64(live)
	}
	return 1 / (1 + float64(live)/float64(backgroundCPU))
}

func pressureScale(level PressureLevel) float64 {
	switch level {
	case PressureCritical:
		return 0.12
	case PressureHigh:
		return 0.28
	case PressureModerate:
		return 0.62
	default:
		return 1
	}
}

func cpuIdleRatio(snapshot runtimeSnapshot) float64 {
	if snapshot.SystemCPU <= 0 {
		return 0.55
	}
	return clampRatio((100 - snapshot.SystemCPU) / 100)
}

func memoryFreeRatio(snapshot runtimeSnapshot) float64 {
	if snapshot.SystemTotal == 0 {
		return 0.55
	}
	return clampRatio(float64(snapshot.SystemTotal-snapshot.SystemUsed) / float64(snapshot.SystemTotal))
}

func memoryUsedPercent(snapshot runtimeSnapshot) float64 {
	if snapshot.SystemTotal == 0 {
		return 0
	}
	return clampRatio(float64(snapshot.SystemUsed)/float64(snapshot.SystemTotal)) * 100
}

func uiReserveRatio(snapshot runtimeSnapshot) float64 {
	idle := cpuIdleRatio(snapshot)
	switch {
	case idle < 0.12:
		return 0.28
	case idle < 0.35:
		return 0.18
	default:
		return 0.08
	}
}

func adaptiveHTTPIdleConnections(networkWorkers int, snapshot runtimeSnapshot) int {
	return positiveCeil(float64(networkWorkers) * (1.2 + memoryFreeRatio(snapshot)))
}

func adaptiveHTTPIdlePerHost(networkWorkers int, snapshot runtimeSnapshot) int {
	return positiveCeil(math.Sqrt(float64(maxInt(1, networkWorkers))) * (1 + memoryFreeRatio(snapshot)))
}

func adaptiveHTTPRateLimit(networkWorkers int, snapshot runtimeSnapshot) int {
	return positiveCeil(float64(networkWorkers) * (1 + cpuIdleRatio(snapshot) + memoryFreeRatio(snapshot)))
}

func adaptiveMemoryCacheBudget(networkWorkers int, snapshot runtimeSnapshot) int64 {
	if snapshot.SystemTotal == 0 {
		return int64(maxInt(1, networkWorkers)) * int64(pageSize()) * 2048
	}
	free := snapshot.SystemTotal - snapshot.SystemUsed
	share := float64(free) * (0.02 + 0.04*memoryFreeRatio(snapshot))
	perWorker := float64(maxInt(1, networkWorkers)) * float64(pageSize()) * 128
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

func capByMaxWorkers(workers int, maxWorkers int) int {
	if maxWorkers <= 0 {
		return workers
	}
	return minPositive(workers, maxWorkers)
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
