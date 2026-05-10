package phygoboost

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	systemcpu "github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/tklauser/numcpus"
)

func workerEnv(kind workKind) []string {
	profile := Current()
	gomax := workerGOMAXPROCS(kind, profile.BackgroundCPU, profile.ProcessWorkers)
	childNetworkWorkers := childWorkerNetworkWorkers(kind, profile)
	childDiskWorkers := childWorkerDiskWorkers(kind, profile)
	childProcessWorkers := childWorkerProcessWorkers(profile)
	nativeThreads := externalNativeThreads(gomax, profile)
	env := []string{
		"GOMAXPROCS=" + strconv.Itoa(gomax),
		"PHYTOZOME_GO_MAX_WORKERS=" + strconv.Itoa(childNetworkWorkers),
		"PHYTOZOME_GO_NETWORK_WORKERS=" + strconv.Itoa(childNetworkWorkers),
		"PHYTOZOME_GO_DISK_WORKERS=" + strconv.Itoa(childDiskWorkers),
		"PHYTOZOME_GO_PROCESS_WORKERS=" + strconv.Itoa(childProcessWorkers),
		"PHYTOZOME_GO_HTTP_RATE_LIMIT=" + strconv.Itoa(maxInt(2, childNetworkWorkers*2)),
		"PHYTOZOME_GO_WORKER_PRESSURE=" + pressureName(RuntimeState().Pressure()),
		"PHYTOZOME_GO_WORKER_ACTIVE=" + strconv.Itoa(profile.ActiveWorkers),
		"PHYTOZOME_GO_WORKER_CHILDREN=" + strconv.Itoa(profile.ChildProcesses),
		"OMP_NUM_THREADS=" + strconv.Itoa(nativeThreads),
		"OPENBLAS_NUM_THREADS=" + strconv.Itoa(nativeThreads),
		"MKL_NUM_THREADS=" + strconv.Itoa(nativeThreads),
		"NUMEXPR_NUM_THREADS=" + strconv.Itoa(nativeThreads),
	}
	if limit := workerMemoryLimitBytes(profile.MemoryHardLimit, profile.ProcessWorkers, profile.ActiveWorkers, profile.ChildProcesses); limit > 0 {
		env = append(env, "GOMEMLIMIT="+strconv.FormatUint(limit, 10))
	}
	return env
}

func workerGOMAXPROCS(kind workKind, cpu int, workers int) int {
	if env := configuredInt("PHYTOZOME_GO_WORKER_GOMAXPROCS", 0); env > 0 {
		return env
	}
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	if workers < 1 {
		workers = 1
	}
	switch kind {
	case workCPU, workProcess:
		return positiveCeil(float64(cpu) / float64(workers))
	case workDisk:
		return positiveCeil(float64(cpu) / math.Sqrt(float64(workers+1)))
	default:
		return positiveCeil(float64(cpu) / float64(workers+1))
	}
}

func workerMemoryLimitBytes(hard uint64, processWorkers int, activeWorkers int, childProcesses int) uint64 {
	if env := configuredInt64("PHYTOZOME_GO_WORKER_MEMORY_BYTES", 0); env > 0 {
		return uint64(env)
	}
	if hard == 0 {
		_, hard = memoryLimits()
	}
	shares := maxInt(1, processWorkers)
	live := activeWorkers + childProcesses
	if live > shares {
		shares = live
	}
	limit := hard / uint64(shares+1)
	minLimit := uint64(128 * 1024 * 1024)
	if limit < minLimit {
		return minLimit
	}
	return limit
}

func pressureName(level PressureLevel) string {
	switch level {
	case PressureCritical:
		return "critical"
	case PressureHigh:
		return "high"
	case PressureModerate:
		return "moderate"
	default:
		return "low"
	}
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

func configuredInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func configuredDurationMS(name string, fallback time.Duration) time.Duration {
	value := configuredInt(name, 0)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Millisecond
}

func configuredDurationSeconds(name string, fallback time.Duration) time.Duration {
	value := configuredInt(name, 0)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func configuredBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func workerProcessMode() bool {
	return strings.TrimSpace(os.Getenv("PHYTOZOME_GO_WORKER")) == "1"
}

func activeCPUCount() int {
	if configured := runtime.GOMAXPROCS(0); configured > 0 {
		if online, err := numcpus.GetOnline(); err == nil && online > 0 && online < configured {
			return online
		}
		return configured
	}
	if online, err := numcpus.GetOnline(); err == nil && online > 0 {
		return online
	}
	return runtime.NumCPU()
}

func sampleSystemMemory() (uint64, uint64, uint64) {
	var rss uint64
	if proc, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if info, err := proc.MemoryInfo(); err == nil && info != nil {
			rss = info.RSS
		}
	}
	var used uint64
	var total uint64
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		used = vm.Used
		total = vm.Total
	}
	return rss, used, total
}

func sampleSystemCPUPercent() float64 {
	values, err := systemcpu.Percent(0, false)
	if err != nil || len(values) == 0 {
		return 0
	}
	if values[0] < 0 {
		return 0
	}
	if values[0] > 100 {
		return 100
	}
	return values[0]
}

func memoryLimits() (uint64, uint64) {
	soft := uint64(configuredInt64("PHYTOZOME_GO_MEMORY_SOFT_BYTES", 0))
	hard := uint64(configuredInt64("PHYTOZOME_GO_MEMORY_HARD_BYTES", 0))
	if soft == 0 {
		soft = defaultMemorySoftLimit()
	}
	if hard == 0 {
		hard = soft * 3 / 2
	}
	if hard < soft {
		hard = soft
	}
	return soft, hard
}

func defaultMemorySoftLimit() uint64 {
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil && vm.Total > 0 {
		limit := vm.Total / 4
		minLimit := uint64(512 * 1024 * 1024)
		maxLimit := uint64(4 * 1024 * 1024 * 1024)
		if limit < minLimit {
			return minLimit
		}
		if limit > maxLimit {
			return maxLimit
		}
		return limit
	}
	return 768 * 1024 * 1024
}
