package perf

import (
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Profile struct {
	CPU             int
	NetworkWorkers  int
	DiskWorkers     int
	MaxIdleConns    int
	MaxIdlePerHost  int
	UIThrottle      time.Duration
	UIAnimationTick time.Duration
	SearchDebounce  time.Duration
	HTTPTimeout     time.Duration
	IdleConnTimeout time.Duration
	TLSHandshake    time.Duration
	ExpectContinue  time.Duration
}

func Current() Profile {
	cpu := runtime.GOMAXPROCS(0)
	if cpu < 1 {
		cpu = runtime.NumCPU()
	}
	maxWorkers := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0)
	networkWorkers := maxInt(cpu*16, 96)
	if maxWorkers > networkWorkers {
		networkWorkers = maxWorkers
	}
	diskWorkers := maxInt(2, minInt(cpu, 8))
	if envDisk := configuredInt("PHYTOZOME_GO_DISK_WORKERS", 0); envDisk > 0 {
		diskWorkers = envDisk
	}
	maxIdle := configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS", maxInt(networkWorkers*2, 512))
	return Profile{
		CPU:             cpu,
		NetworkWorkers:  networkWorkers,
		DiskWorkers:     diskWorkers,
		MaxIdleConns:    maxIdle,
		MaxIdlePerHost:  configuredInt("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST", maxInt(networkWorkers, 128)),
		UIThrottle:      configuredDurationMS("PHYTOZOME_GO_UI_THROTTLE_MS", 80*time.Millisecond),
		UIAnimationTick: configuredDurationMS("PHYTOZOME_GO_UI_ANIMATION_MS", 120*time.Millisecond),
		SearchDebounce:  configuredDurationMS("PHYTOZOME_GO_SEARCH_DEBOUNCE_MS", 35*time.Millisecond),
		HTTPTimeout:     configuredDurationSeconds("PHYTOZOME_GO_HTTP_TIMEOUT_SECONDS", 60*time.Second),
		IdleConnTimeout: configuredDurationSeconds("PHYTOZOME_GO_HTTP_IDLE_SECONDS", 90*time.Second),
		TLSHandshake:    configuredDurationSeconds("PHYTOZOME_GO_HTTP_TLS_SECONDS", 10*time.Second),
		ExpectContinue:  configuredDurationSeconds("PHYTOZOME_GO_HTTP_EXPECT_SECONDS", time.Second),
	}
}

func HTTPClient() *http.Client {
	profile := Current()
	return &http.Client{
		Timeout: profile.HTTPTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: profile.TLSHandshake, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          profile.MaxIdleConns,
			MaxIdleConnsPerHost:   profile.MaxIdlePerHost,
			IdleConnTimeout:       profile.IdleConnTimeout,
			TLSHandshakeTimeout:   profile.TLSHandshake,
			ExpectContinueTimeout: profile.ExpectContinue,
		},
	}
}

func DynamicWorkers(total int, preferredMax int) int {
	if total <= 0 {
		return 0
	}
	profile := Current()
	cpu := profile.CPU
	limit := preferredMax
	if limit <= 0 {
		limit = profile.NetworkWorkers
	}
	if limit < cpu {
		limit = cpu
	}
	if envLimit := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envLimit > 0 && envLimit > limit {
		limit = envLimit
	}
	if total < limit {
		return total
	}
	return limit
}

func CPUWorkers(total int) int {
	return DynamicWorkers(total, Current().CPU)
}

func DiskWorkers(total int) int {
	return DynamicWorkers(total, Current().DiskWorkers)
}

func NetworkWorkers(total int) int {
	return DynamicWorkers(total, Current().NetworkWorkers)
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
