package phygoboost

import (
	"context"

	"golang.org/x/sync/singleflight"
)

// SingleflightDoContext waits for a shared singleflight call while still letting
// the caller stop waiting when its context is cancelled.
func SingleflightDoContext(ctx context.Context, group *singleflight.Group, key string, fn func() (any, error)) (any, error, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return nil, nil, false
	}
	if group == nil {
		value, err := fn()
		return value, err, false
	}

	result := group.DoChan(key, fn)
	select {
	case reply := <-result:
		return reply.Val, reply.Err, reply.Shared
	case <-ctx.Done():
		group.Forget(key)
		return nil, ctx.Err(), false
	}
}
