package phygoboost

import (
	"context"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

type PressureLevel int

const (
	PressureLow PressureLevel = iota
	PressureModerate
	PressureHigh
	PressureCritical
)

type State struct {
	mu             sync.RWMutex
	stats          runtime.MemStats
	processRSS     uint64
	systemUsed     uint64
	systemTotal    uint64
	systemCPU      float64
	childProcesses int
	pressure       PressureLevel
	softLimit      uint64
	hardLimit      uint64
	lastSample     time.Time
	lastTrim       time.Time
	cleaners       []func()
	cacheBytes     atomic.Int64
	activeWorkers  atomic.Int64
}

var (
	runtimeOnce  sync.Once
	runtimeState *State
)

func RuntimeState() *State {
	runtimeOnce.Do(func() {
		soft, hard := memoryLimits()
		runtimeState = &State{softLimit: soft, hardLimit: hard}
		runtimeState.sample()
		go runtimeState.loop()
	})
	return runtimeState
}

func RegisterCleaner(cleaner func()) {
	if cleaner == nil {
		return
	}
	state := RuntimeState()
	state.mu.Lock()
	state.cleaners = append(state.cleaners, cleaner)
	state.mu.Unlock()
}

func ObserveCacheBytes(n int64) {
	if n <= 0 {
		return
	}
	RuntimeState().cacheBytes.Add(n)
	RuntimeState().maybeTrim()
}

func (s *State) Pressure() PressureLevel {
	s.sampleIfStale()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pressure
}

func (s *State) ChildProcesses() int {
	s.sampleIfStale()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.childProcesses
}

func (s *State) systemCPUPercentSnapshot() float64 {
	s.sampleIfStale()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.systemCPU
}

func (s *State) snapshot() runtimeSnapshot {
	s.sampleIfStale()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return runtimeSnapshot{
		ProcessRSS:     s.processRSS,
		SystemUsed:     s.systemUsed,
		SystemTotal:    s.systemTotal,
		SystemCPU:      s.systemCPU,
		ChildProcesses: s.childProcesses,
		ActiveWorkers:  int(s.activeWorkers.Load()),
		Pressure:       s.pressure,
	}
}

func sampleChildProcessCount() int {
	parent := os.Getpid()
	children := 0
	procs, err := process.Processes()
	if err != nil {
		return 0
	}
	for _, proc := range procs {
		if proc == nil {
			continue
		}
		ppid, err := proc.Ppid()
		if err == nil && int(ppid) == parent {
			children++
		}
	}
	return children
}

func (s *State) loop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.sample()
		s.maybeTrim()
	}
}

func (s *State) sampleIfStale() {
	s.mu.RLock()
	stale := time.Since(s.lastSample) > 750*time.Millisecond
	s.mu.RUnlock()
	if stale {
		s.sample()
	}
}

func (s *State) sample() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	processRSS, systemUsed, systemTotal := sampleSystemMemory()
	systemCPU := sampleSystemCPUPercent()
	childProcesses := sampleChildProcessCount()
	pressure := classifyPressure(stats, processRSS, systemUsed, systemTotal, s.softLimit, s.hardLimit)
	s.mu.Lock()
	s.stats = stats
	s.processRSS = processRSS
	s.systemUsed = systemUsed
	s.systemTotal = systemTotal
	s.systemCPU = systemCPU
	s.childProcesses = childProcesses
	s.pressure = pressure
	s.lastSample = time.Now()
	s.mu.Unlock()
}

func (s *State) maybeTrim() {
	pressure := s.Pressure()
	if pressure < PressureHigh {
		return
	}
	s.mu.Lock()
	if time.Since(s.lastTrim) < 10*time.Second {
		s.mu.Unlock()
		return
	}
	s.lastTrim = time.Now()
	cleaners := append([]func(){}, s.cleaners...)
	s.mu.Unlock()

	for _, cleaner := range cleaners {
		cleaner()
	}
	debug.FreeOSMemory()
}

func classifyPressure(stats runtime.MemStats, processRSS uint64, systemUsed uint64, systemTotal uint64, soft uint64, hard uint64) PressureLevel {
	alloc := maxUint64(stats.Alloc, processRSS)
	switch {
	case hard > 0 && alloc >= hard:
		return PressureCritical
	case systemTotal > 0 && systemUsed*100/systemTotal >= 92:
		return PressureCritical
	case soft > 0 && alloc >= soft:
		return PressureHigh
	case systemTotal > 0 && systemUsed*100/systemTotal >= 84:
		return PressureHigh
	case soft > 0 && alloc >= soft*75/100:
		return PressureModerate
	case systemTotal > 0 && systemUsed*100/systemTotal >= 75:
		return PressureModerate
	case stats.NumGC > 0 && stats.PauseTotalNs > uint64(5*time.Second):
		return PressureModerate
	default:
		return PressureLow
	}
}

func WorkerStarted() func() {
	state := RuntimeState()
	state.activeWorkers.Add(1)
	return func() {
		state.activeWorkers.Add(-1)
	}
}

func ObserveWork(kind workKind, duration time.Duration, queueDelay time.Duration, err error) {
	_ = kind
	_ = duration
	_ = queueDelay
	_ = err
}

func ObserveTaskSpec(ctx context.Context, spec TaskSpec, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return nil
	}
	started := time.Now()
	err := fn(ctx)
	ObserveWork(workKindForTaskSpec(spec), time.Since(started), 0, err)
	return err
}

func DrainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64*1024))
	_ = body.Close()
}
