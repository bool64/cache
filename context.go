package cache

import (
	"context"
	"time"
)

type (
	skipReadCtxKey struct{}
	ttlCtxKey      struct{}
)

// WithTTL adds cache time to live information to context.
//
// If there is already ttl in context and updateExisting, then ttl value in original context will be updated.
//
// Updating existing ttl can be useful if ttl information is only available internally during cache value build,
// in such a case value builder can indicate ttl to external cache backend.
// For example cache ttl can be derived from HTTP response cache headers of value source.
//
// When existing ttl is updated minimal non-zero value is kept.
// This is necessary to unite ttl requirements of multiple parties.
func WithTTL(ctx context.Context, ttl time.Duration, updateExisting bool) context.Context {
	if updateExisting {
		if existing, ok := ctx.Value(ttlCtxKey{}).(*time.Duration); ok {
			if *existing == 0 || *existing > ttl {
				*existing = ttl
			}

			return ctx
		}
	}

	return context.WithValue(ctx, ttlCtxKey{}, &ttl)
}

// TTL retrieves cache time to live from context, zero value is returned by default.
func TTL(ctx context.Context) time.Duration {
	ttl, ok := ctx.Value(ttlCtxKey{}).(*time.Duration)
	if ok {
		return *ttl
	}

	return 0
}

// WithSkipRead returns context with cache read ignored.
//
// With such context cache.Reader should always return ErrNotFound discarding cached value.
func WithSkipRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipReadCtxKey{}, true)
}

// SkipRead returns true if cache read is ignored in context.
func SkipRead(ctx context.Context) bool {
	_, ok := ctx.Value(skipReadCtxKey{}).(bool)

	return ok
}

// detachedContext exposes parent values, but suppresses parent cancellation.
type detachedContext struct {
	parent context.Context
}

func (d detachedContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (d detachedContext) Done() <-chan struct{} {
	return nil
}

func (d detachedContext) Err() error {
	return nil
}

func (d detachedContext) Value(key interface{}) interface{} {
	return d.parent.Value(key)
}
