package phygoboost

import (
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
