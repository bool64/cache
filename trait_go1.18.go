//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"time"
)

type traitOf[V any] struct {
	trait
}

func newTraitOf[V any](b backend, config Config) *traitOf[V] {
	t := &traitOf[V]{}

	t.trait = *newTrait(b, config)

	return t
}

func (c *traitOf[V]) prepareRead(ctx context.Context, cacheEntry *entryOf[V], found bool) (v V, err error) {
	if !found {
		if c.logDebug != nil {
			c.logDebug(ctx, "cache miss", "name", c.Config.Name)
		}

		if c.stat != nil {
			c.stat.Add(ctx, MetricMiss, 1, "name", c.Config.Name)
		}

		return v, ErrNotFound
	}

	if cacheEntry.E.Before(time.Now()) {
		if c.logDebug != nil {
			c.logDebug(ctx, "cache key expired", "name", c.Config.Name)
		}

		if c.stat != nil {
			c.stat.Add(ctx, MetricExpired, 1, "name", c.Config.Name)
		}

		return v, errExpiredOf[V]{entry: cacheEntry}
	}

	if c.stat != nil {
		c.stat.Add(ctx, MetricHit, 1, "name", c.Config.Name)
	}

	if c.logDebug != nil {
		c.logDebug(ctx, "cache hit",
			"name", c.Config.Name,
			"entry", cacheEntry,
		)
	}

	return cacheEntry.V, nil
}

// entry is a cache entry.
type entryOf[V any] struct {
	K keyString `json:"key"`
	V V         `json:"val"`
	E time.Time `json:"exp"`
}

func (e entryOf[V]) Key() []byte {
	return e.K
}

func (e entryOf[V]) Value() V {
	return e.V
}

func (e entryOf[V]) ExpireAt() time.Time {
	return e.E
}

var _ ErrWithExpiredItemOf[any] = errExpiredOf[any]{}

type errExpiredOf[V any] struct {
	entry *entryOf[V]
}

func (e errExpiredOf[V]) Error() string {
	return ErrExpired.Error()
}

func (e errExpiredOf[V]) Value() V {
	return e.entry.V
}

func (e errExpiredOf[V]) ExpiredAt() time.Time {
	return e.entry.E
}

func (e errExpiredOf[V]) Is(err error) bool {
	return err == ErrExpired // nolint:errorlint,goerr113  // Target sentinel error is not wrapped.
}
