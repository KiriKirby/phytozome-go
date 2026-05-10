package network

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DomainGrant struct {
	Domain   string
	Slots    int
	Acquired time.Time
}

type DomainSnapshot struct {
	Domain        string
	Limit         int
	Active        int
	Waiting       int
	CooldownUntil time.Time
	LastLatency   time.Duration
	SuccessWindow int
	FailureWindow int
	RateLimited   int
}

type Manager struct {
	mu           sync.Mutex
	client       *http.Client
	globalLimit  func() int
	pools        map[string]*domainPool
	hostOverride func(string) string
}

type domainPool struct {
	domain        string
	active        int
	waiting       []*domainWaiter
	limit         int
	cooldownUntil time.Time
	lastLatency   time.Duration
	successWindow int
	failureWindow int
	rateLimited   int
}

type domainWaiter struct {
	slots int
	ready chan *DomainGrant
}

func NewManager(client *http.Client, globalLimit func() int) *Manager {
	if client == nil {
		client = &http.Client{}
	}
	if globalLimit == nil {
		globalLimit = func() int { return 2 }
	}
	return &Manager{
		client:      client,
		globalLimit: globalLimit,
		pools:       make(map[string]*domainPool),
	}
}

func (m *Manager) HTTPClient() *http.Client {
	if m == nil || m.client == nil {
		return &http.Client{}
	}
	return m.client
}

func (m *Manager) Acquire(ctx context.Context, domain string, slots int) (*DomainGrant, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if slots < 1 {
		slots = 1
	}
	m.mu.Lock()
	pool := m.poolLocked(domain)
	now := time.Now()
	if len(pool.waiting) == 0 && now.After(pool.cooldownUntil) && pool.active+slots <= pool.currentLimit(m.limitLocked()) {
		pool.active += slots
		grant := &DomainGrant{Domain: pool.domain, Slots: slots, Acquired: now}
		m.mu.Unlock()
		return grant, nil
	}
	waiter := &domainWaiter{slots: slots, ready: make(chan *DomainGrant, 1)}
	pool.waiting = append(pool.waiting, waiter)
	m.mu.Unlock()
	select {
	case grant := <-waiter.ready:
		return grant, nil
	case <-ctx.Done():
		m.mu.Lock()
		pool.removeWaiter(waiter)
		m.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (m *Manager) Release(grant *DomainGrant, latency time.Duration, err error, rateLimited bool) {
	if m == nil || grant == nil {
		return
	}
	m.mu.Lock()
	pool := m.poolLocked(grant.Domain)
	pool.active -= grant.Slots
	if pool.active < 0 {
		pool.active = 0
	}
	pool.observe(latency, err, rateLimited)
	pool.drain(m.limitLocked())
	m.mu.Unlock()
}

func (m *Manager) Snapshot() []DomainSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DomainSnapshot, 0, len(m.pools))
	globalLimit := m.limitLocked()
	for _, pool := range m.pools {
		if pool == nil {
			continue
		}
		out = append(out, DomainSnapshot{
			Domain:        pool.domain,
			Limit:         pool.currentLimit(globalLimit),
			Active:        pool.active,
			Waiting:       len(pool.waiting),
			CooldownUntil: pool.cooldownUntil,
			LastLatency:   pool.lastLatency,
			SuccessWindow: pool.successWindow,
			FailureWindow: pool.failureWindow,
			RateLimited:   pool.rateLimited,
		})
	}
	return out
}

func (m *Manager) limitLocked() int {
	limit := 2
	if m.globalLimit != nil {
		limit = m.globalLimit()
	}
	if limit < 1 {
		return 1
	}
	return limit
}

func (m *Manager) poolLocked(domain string) *domainPool {
	key := normalizeDomain(domain)
	pool := m.pools[key]
	if pool != nil {
		return pool
	}
	pool = &domainPool{domain: key, limit: 1}
	m.pools[key] = pool
	return pool
}

func (p *domainPool) currentLimit(globalLimit int) int {
	limit := p.limit
	if limit < 1 {
		limit = 1
	}
	if globalLimit < 1 {
		globalLimit = 1
	}
	if limit > globalLimit {
		limit = globalLimit
	}
	return limit
}

func (p *domainPool) removeWaiter(target *domainWaiter) {
	for i := range p.waiting {
		if p.waiting[i] == target {
			p.waiting = append(p.waiting[:i], p.waiting[i+1:]...)
			return
		}
	}
}

func (p *domainPool) drain(globalLimit int) {
	if time.Now().Before(p.cooldownUntil) {
		return
	}
	limit := p.currentLimit(globalLimit)
	for len(p.waiting) > 0 {
		next := p.waiting[0]
		if next == nil {
			p.waiting = p.waiting[1:]
			continue
		}
		if p.active+next.slots > limit {
			return
		}
		p.waiting = p.waiting[1:]
		p.active += next.slots
		next.ready <- &DomainGrant{Domain: p.domain, Slots: next.slots, Acquired: time.Now()}
	}
}

func (p *domainPool) observe(latency time.Duration, err error, rateLimited bool) {
	if latency > 0 {
		p.lastLatency = latency
	}
	if err == nil && !rateLimited {
		p.successWindow++
		if p.failureWindow > 0 {
			p.failureWindow--
		}
		if p.rateLimited > 0 {
			p.rateLimited--
		}
		if p.successWindow >= 4 {
			p.successWindow = 0
			p.limit++
		}
		return
	}
	p.successWindow = 0
	p.failureWindow++
	if rateLimited {
		p.rateLimited++
		if p.limit > 1 {
			p.limit--
		}
		p.cooldownUntil = time.Now().Add(1500 * time.Millisecond)
		return
	}
	if p.limit > 1 {
		p.limit--
	}
	p.cooldownUntil = time.Now().Add(500 * time.Millisecond)
}

func normalizeDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	if idx := strings.Index(domain, "/"); idx >= 0 {
		domain = domain[:idx]
	}
	if domain == "" {
		return "unknown"
	}
	return domain
}
