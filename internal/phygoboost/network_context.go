package phygoboost

import (
	"context"
	"strings"
)

type activeNetworkGrantKey struct{}
type activeLocalGrantKey struct{}

func contextWithNetworkGrants(ctx context.Context, grants []*NetworkGrant) context.Context {
	if ctx == nil || len(grants) == 0 {
		return ctx
	}
	current, _ := ctx.Value(activeNetworkGrantKey{}).(map[string]int)
	next := make(map[string]int, len(current)+len(grants))
	for domain, count := range current {
		if strings.TrimSpace(domain) != "" && count > 0 {
			next[domain] = count
		}
	}
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		domain := strings.TrimSpace(grant.Domain)
		if domain == "" {
			continue
		}
		next[domain] += maxInt(1, grant.Slots)
	}
	if len(next) == 0 {
		return ctx
	}
	return context.WithValue(ctx, activeNetworkGrantKey{}, next)
}

func contextHasNetworkGrant(ctx context.Context, domain string) bool {
	if ctx == nil {
		return false
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return false
	}
	values, _ := ctx.Value(activeNetworkGrantKey{}).(map[string]int)
	return values[domain] > 0
}

func contextLocalGrant(ctx context.Context) (*LocalGrant, bool) {
	if ctx == nil {
		return nil, false
	}
	grant, ok := ctx.Value(activeLocalGrantKey{}).(*LocalGrant)
	if !ok || grant == nil {
		return nil, false
	}
	return grant, true
}

func contextWithLocalGrant(ctx context.Context, grant *LocalGrant) context.Context {
	if ctx == nil || grant == nil {
		return ctx
	}
	return context.WithValue(ctx, activeLocalGrantKey{}, grant)
}

func networkGrantSnapshotFromContext(ctx context.Context) map[string]int {
	if ctx == nil {
		return nil
	}
	values, _ := ctx.Value(activeNetworkGrantKey{}).(map[string]int)
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for domain, count := range values {
		domain = strings.TrimSpace(domain)
		if domain == "" || count <= 0 {
			continue
		}
		out[domain] = count
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func contextWithNetworkGrantSnapshot(ctx context.Context, grants map[string]int) context.Context {
	if ctx == nil || len(grants) == 0 {
		return ctx
	}
	next := make(map[string]int, len(grants))
	for domain, count := range grants {
		domain = strings.TrimSpace(domain)
		if domain == "" || count <= 0 {
			continue
		}
		next[domain] = count
	}
	if len(next) == 0 {
		return ctx
	}
	return context.WithValue(ctx, activeNetworkGrantKey{}, next)
}
