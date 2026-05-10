package phygoboost

import (
	"context"
	"net/http"
	"sync"
	"time"

	boostcore "github.com/KiriKirby/phytozome-go/internal/phygoboost/core"
	boostnetwork "github.com/KiriKirby/phytozome-go/internal/phygoboost/network"
)

type runtimeCoordinator struct {
	localScheduler  *boostcore.Scheduler
	networkManager  *boostnetwork.Manager
	sharedHTTP      *http.Client
	sharedHTTPOnce  sync.Once
	networkOnce     sync.Once
	localOnce       sync.Once
}

var (
	coordinatorOnce sync.Once
	coordinatorInst *runtimeCoordinator
)

func coordinator() *runtimeCoordinator {
	coordinatorOnce.Do(func() {
		coordinatorInst = &runtimeCoordinator{}
	})
	return coordinatorInst
}

func (r *runtimeCoordinator) local() *boostcore.Scheduler {
	r.localOnce.Do(func() {
		r.localScheduler = boostcore.NewScheduler(func() int {
			profile := Current()
			if profile.BackgroundCPU < 1 {
				return 1
			}
			capacity := profile.BackgroundCPU
			if profile.MemoryUsedPercent > 88 {
				capacity /= 2
			}
			if capacity < 1 {
				capacity = 1
			}
			return capacity
		})
	})
	return r.localScheduler
}

func (r *runtimeCoordinator) httpClient() *http.Client {
	r.sharedHTTPOnce.Do(func() {
		r.sharedHTTP = newSharedHTTPClient()
	})
	return r.sharedHTTP
}

func (r *runtimeCoordinator) network() *boostnetwork.Manager {
	r.networkOnce.Do(func() {
		r.networkManager = boostnetwork.NewManager(r.httpClient(), func() int {
			profile := Current()
			limit := profile.NetworkWorkers
			if limit < 1 {
				limit = 1
			}
			if profile.MemoryUsedPercent > 88 {
				limit /= 2
			}
			if limit < 1 {
				limit = 1
			}
			return limit
		})
	})
	return r.networkManager
}

func AcquireLocal(ctx context.Context, level ExecLevel, slots int) (*LocalGrant, error) {
	grant, err := coordinator().local().Acquire(ctx, boostcore.Level(level), slots)
	if err != nil {
		return nil, err
	}
	return &LocalGrant{
		ID:       grant.ID,
		Level:    ExecLevel(grant.Level),
		Slots:    grant.Slots,
		Acquired: grant.Acquired,
	}, nil
}

func ReleaseLocal(grant *LocalGrant) {
	if grant == nil {
		return
	}
	coordinator().local().Release(&boostcore.LocalGrant{
		ID:       grant.ID,
		Level:    boostcore.Level(grant.Level),
		Slots:    grant.Slots,
		Acquired: grant.Acquired,
	})
}

func AcquireNetwork(ctx context.Context, domain string, slots int) (*NetworkGrant, func(time.Duration, error, bool), error) {
	grant, err := coordinator().network().Acquire(ctx, domain, slots)
	if err != nil {
		return nil, nil, err
	}
	out := &NetworkGrant{
		Domain:   grant.Domain,
		Slots:    grant.Slots,
		Acquired: grant.Acquired,
	}
	release := func(latency time.Duration, err error, rateLimited bool) {
		coordinator().network().Release(grant, latency, err, rateLimited)
	}
	return out, release, nil
}

func HTTPClientForDomain(domain string) *http.Client {
	_ = domain
	return coordinator().network().HTTPClient()
}

func DeclareResources(ctx context.Context, request ResourceRequest) (*ResourceHandle, error) {
	handle := &ResourceHandle{}
	if request.LocalSlots > 0 {
		grant, err := AcquireLocal(ctx, request.LocalLevel, request.LocalSlots)
		if err != nil {
			return nil, err
		}
		handle.local = grant
	}
	for domain, slots := range request.Network {
		grant, release, err := AcquireNetwork(ctx, domain, slots)
		if err != nil {
			handle.Release()
			return nil, err
		}
		handle.networks = append(handle.networks, grant)
		handle.releasers = append(handle.releasers, release)
	}
	return handle, nil
}

func BindDeclaredResources(ctx context.Context, handle *ResourceHandle) context.Context {
	if ctx == nil || handle == nil {
		return ctx
	}
	if handle.local != nil {
		ctx = contextWithLocalGrant(ctx, handle.local)
	}
	if len(handle.networks) > 0 {
		ctx = contextWithNetworkGrants(ctx, handle.networks)
	}
	return ctx
}

func (h *ResourceHandle) Release() {
	if h == nil || h.released {
		return
	}
	h.released = true
	for i := len(h.releasers) - 1; i >= 0; i-- {
		if h.releasers[i] != nil {
			h.releasers[i](0, nil, false)
		}
	}
	if h.local != nil {
		ReleaseLocal(h.local)
	}
}
