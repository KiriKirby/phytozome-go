package phygoboost

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
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
	workFeedback   map[workKind]workFeedback
}

type workFeedback struct {
	Count          uint64
	Failures       uint64
	DurationEWMA   float64
	QueueEWMA      float64
	ThroughputEWMA float64
	LastDuration   float64
	LastQueue      float64
	LastObserved   time.Time
}

type WorkFeedback struct {
	Count          uint64
	Failures       uint64
	Duration       time.Duration
	QueueDelay     time.Duration
	Throughput     float64
	LastDuration   time.Duration
	LastQueueDelay time.Duration
	LastObserved   time.Time
}

var (
	runtimeOnce  sync.Once
	runtimeState *State
)

func RuntimeState() *State {
	runtimeOnce.Do(func() {
		soft, hard := memoryLimits()
		runtimeState = &State{softLimit: soft, hardLimit: hard, workFeedback: make(map[workKind]workFeedback)}
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

func (s *State) feedback(kind workKind) workFeedback {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workFeedback[kind]
}

func (s *State) feedbackSnapshot() map[string]WorkFeedback {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.workFeedback) == 0 {
		return nil
	}
	out := make(map[string]WorkFeedback, len(s.workFeedback))
	for kind, feedback := range s.workFeedback {
		out[string(kind)] = WorkFeedback{
			Count:          feedback.Count,
			Failures:       feedback.Failures,
			Duration:       time.Duration(feedback.DurationEWMA),
			QueueDelay:     time.Duration(feedback.QueueEWMA),
			Throughput:     feedback.ThroughputEWMA,
			LastDuration:   time.Duration(feedback.LastDuration),
			LastQueueDelay: time.Duration(feedback.LastQueue),
			LastObserved:   feedback.LastObserved,
		}
	}
	return out
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

func (s *State) AdjustWorkers(kind workKind, base int) int {
	if base < 1 {
		return 1
	}
	snapshot := s.snapshot()
	scale := runtimeScale(kind, snapshot)
	base = positiveCeil(float64(base) * scale)
	if kind == workProcess {
		base = positiveCeil(float64(base) * processContentionScale(snapshot, base))
	}
	return base
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
	state := RuntimeState()
	const alpha = 0.22
	state.mu.Lock()
	feedback := state.workFeedback[kind]
	feedback.Count++
	if isPerformanceFailure(err) {
		feedback.Failures++
	}
	if duration > 0 {
		feedback.DurationEWMA = ewmaDuration(feedback.DurationEWMA, duration, alpha)
		feedback.LastDuration = float64(duration)
		feedback.ThroughputEWMA = ewmaFloat(feedback.ThroughputEWMA, 1/float64(duration.Seconds()), alpha)
	}
	if queueDelay > 0 {
		feedback.QueueEWMA = ewmaDuration(feedback.QueueEWMA, queueDelay, alpha)
		feedback.LastQueue = float64(queueDelay)
	}
	feedback.LastObserved = time.Now()
	state.workFeedback[kind] = feedback
	state.mu.Unlock()
}

func isPerformanceFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	for _, marker := range []string{
		"task cancelled",
		"exit requested",
		"back to ",
	} {
		if strings.Contains(message, marker) {
			return false
		}
	}
	return true
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
