package network

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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
	MaxLimit      int
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
	pools        map[string]*domainPool
	managedLimit func() int
}

type domainPool struct {
	domain        string
	active        int
	waiting       []*domainWaiter
	limit         int
	maxLimit      int
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

func NewManager(client *http.Client, managedLimit func() int) *Manager {
	if client == nil {
		client = &http.Client{}
	}
	return &Manager{
		client:       client,
		pools:        make(map[string]*domainPool),
		managedLimit: managedLimit,
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
	if len(pool.waiting) == 0 && now.After(pool.cooldownUntil) && pool.active+slots <= pool.currentLimit() {
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
	pool.observe(latency, err, rateLimited, 0)
	pool.drain()
	m.mu.Unlock()
}

func (m *Manager) Observe(domain string, latency time.Duration, err error, rateLimited bool, cooldown time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	pool := m.poolLocked(domain)
	pool.observe(latency, err, rateLimited, cooldown)
	pool.drain()
	m.mu.Unlock()
}

func (m *Manager) Snapshot() []DomainSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DomainSnapshot, 0, len(m.pools))
	for _, pool := range m.pools {
		if pool == nil {
			continue
		}
		out = append(out, DomainSnapshot{
			Domain:        pool.domain,
			Limit:         pool.currentLimit(),
			MaxLimit:      pool.currentMaxLimit(),
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

func (m *Manager) poolLocked(domain string) *domainPool {
	key := normalizeDomain(domain)
	pool := m.pools[key]
	if pool != nil {
		pool.syncMaxLimit(m.managedLimit)
		return pool
	}
	pool = &domainPool{domain: key, limit: 1, maxLimit: 4}
	pool.syncMaxLimit(m.managedLimit)
	m.pools[key] = pool
	return pool
}

func (p *domainPool) currentLimit() int {
	limit := p.limit
	if limit < 1 {
		limit = 1
	}
	return limit
}

func (p *domainPool) currentMaxLimit() int {
	if p.maxLimit < 1 {
		return 1
	}
	return p.maxLimit
}

func (p *domainPool) syncMaxLimit(managedLimit func() int) {
	maxLimit := 4
	if managedLimit != nil {
		if limit := managedLimit(); limit > 0 {
			maxLimit = limit
		}
	}
	if maxLimit < 1 {
		maxLimit = 1
	}
	p.maxLimit = maxLimit
	if p.limit > p.maxLimit {
		p.limit = p.maxLimit
	}
}

func (p *domainPool) removeWaiter(target *domainWaiter) {
	for i := range p.waiting {
		if p.waiting[i] == target {
			p.waiting = append(p.waiting[:i], p.waiting[i+1:]...)
			return
		}
	}
}

func (p *domainPool) drain() {
	if time.Now().Before(p.cooldownUntil) {
		return
	}
	limit := p.currentLimit()
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

func (p *domainPool) observe(latency time.Duration, err error, rateLimited bool, cooldown time.Duration) {
	if latency > 0 {
		p.lastLatency = latency
	}
	if err == nil && !rateLimited {
		if p.lastLatency >= 4*time.Second {
			p.successWindow = 0
			p.failureWindow++
			p.limit = max(1, p.limit/2)
			p.cooldownUntil = time.Now().Add(maxDuration(cooldown, time.Second))
			return
		}
		p.successWindow++
		if p.failureWindow > 0 {
			p.failureWindow--
		}
		if p.rateLimited > 0 {
			p.rateLimited--
		}
		if p.lastLatency <= 1500*time.Millisecond && p.successWindow >= 3 && p.limit < p.currentMaxLimit() {
			p.successWindow = 0
			p.limit++
		}
		return
	}
	p.successWindow = 0
	p.failureWindow++
	if rateLimited {
		p.rateLimited++
		p.limit = max(1, p.limit/2)
		p.cooldownUntil = time.Now().Add(maxDuration(cooldown, 2*time.Second))
		return
	}
	if p.lastLatency > 0 && p.lastLatency >= 4*time.Second {
		p.limit = max(1, p.limit/2)
		p.cooldownUntil = time.Now().Add(maxDuration(cooldown, time.Second))
		return
	}
	p.limit = max(1, p.limit-1)
	p.cooldownUntil = time.Now().Add(maxDuration(cooldown, 600*time.Millisecond))
}

func RetryAfterDelay(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func IsRetryableStatus(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
}

func IsRetryableError(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func NormalizeDomainForRuntime(domain string) string {
	return normalizeDomain(domain)
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

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
