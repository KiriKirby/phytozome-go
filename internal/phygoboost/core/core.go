package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Level int

const (
	ExecInline Level = iota
	ExecMain
	ExecHeavy
)

type LocalGrant struct {
	ID       string
	Level    Level
	Slots    int
	Acquired time.Time
}

type Snapshot struct {
	Capacity int
	Main     int
	Heavy    int
	Waiting  int
	Grants   []LocalGrant
}

type Scheduler struct {
	mu         sync.Mutex
	nextID     uint64
	capacityFn func() int
	mainActive int
	heavyActive int
	waiting    []*waiter
	grants     map[string]*LocalGrant
}

type waiter struct {
	id    string
	level Level
	slots int
	ready chan *LocalGrant
}

func NewScheduler(capacityFn func() int) *Scheduler {
	if capacityFn == nil {
		capacityFn = func() int { return 1 }
	}
	return &Scheduler{
		capacityFn: capacityFn,
		grants:     make(map[string]*LocalGrant),
	}
}

func (s *Scheduler) Acquire(ctx context.Context, level Level, slots int) (*LocalGrant, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if slots < 1 {
		slots = 1
	}
	if level == ExecInline {
		level = ExecMain
	}
	s.mu.Lock()
	if len(s.waiting) == 0 {
		if grant, ok := s.tryGrantLocked(level, slots); ok {
			s.mu.Unlock()
			return grant, nil
		}
	}
	waiter := &waiter{
		id:    s.nextGrantIDLocked(),
		level: level,
		slots: slots,
		ready: make(chan *LocalGrant, 1),
	}
	s.waiting = append(s.waiting, waiter)
	s.mu.Unlock()

	select {
	case grant := <-waiter.ready:
		return grant, nil
	case <-ctx.Done():
		s.mu.Lock()
		removed := s.removeWaiterLocked(waiter.id)
		s.mu.Unlock()
		if removed {
			return nil, ctx.Err()
		}
		select {
		case grant := <-waiter.ready:
			s.Release(grant)
		default:
		}
		return nil, ctx.Err()
	}
}

func (s *Scheduler) Release(grant *LocalGrant) {
	if grant == nil {
		return
	}
	s.mu.Lock()
	stored := s.grants[grant.ID]
	if stored == nil {
		s.mu.Unlock()
		return
	}
	delete(s.grants, grant.ID)
	switch stored.Level {
	case ExecHeavy:
		s.heavyActive -= stored.Slots
		if s.heavyActive < 0 {
			s.heavyActive = 0
		}
	default:
		s.mainActive -= stored.Slots
		if s.mainActive < 0 {
			s.mainActive = 0
		}
	}
	s.drainLocked()
	s.mu.Unlock()
}

func (s *Scheduler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants := make([]LocalGrant, 0, len(s.grants))
	for _, grant := range s.grants {
		if grant != nil {
			grants = append(grants, *grant)
		}
	}
	return Snapshot{
		Capacity: s.capacityLocked(),
		Main:     s.mainActive,
		Heavy:    s.heavyActive,
		Waiting:  len(s.waiting),
		Grants:   grants,
	}
}

func (s *Scheduler) nextGrantIDLocked() string {
	s.nextID++
	return fmt.Sprintf("local-%d", s.nextID)
}

func (s *Scheduler) capacityLocked() int {
	capacity := 1
	if s.capacityFn != nil {
		capacity = s.capacityFn()
	}
	if capacity < 1 {
		return 1
	}
	return capacity
}

func (s *Scheduler) usedLocked() int {
	return s.mainActive + s.heavyActive
}

func (s *Scheduler) tryGrantLocked(level Level, slots int) (*LocalGrant, bool) {
	if slots < 1 {
		slots = 1
	}
	if s.usedLocked()+slots > s.capacityLocked() {
		return nil, false
	}
	grant := &LocalGrant{
		ID:       s.nextGrantIDLocked(),
		Level:    level,
		Slots:    slots,
		Acquired: time.Now(),
	}
	s.grants[grant.ID] = grant
	switch level {
	case ExecHeavy:
		s.heavyActive += slots
	default:
		s.mainActive += slots
	}
	return grant, true
}

func (s *Scheduler) drainLocked() {
	for len(s.waiting) > 0 {
		next := s.waiting[0]
		grant, ok := s.tryGrantLocked(next.level, next.slots)
		if !ok {
			return
		}
		s.waiting = s.waiting[1:]
		next.ready <- grant
	}
}

func (s *Scheduler) removeWaiterLocked(id string) bool {
	for i := range s.waiting {
		if s.waiting[i] != nil && s.waiting[i].id == id {
			s.waiting = append(s.waiting[:i], s.waiting[i+1:]...)
			return true
		}
	}
	return false
}
