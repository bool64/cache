//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"sync/atomic"
	"time"
)

// TraitBy is a parametrized shared trait, useful to implement ReadWriterBy.
type TraitBy[K comparable, V any] struct {
	Trait
}

// NewTraitBy instantiates new TraitBy.
func NewTraitBy[K comparable, V any](config Config, options ...func(t *Trait)) *TraitBy[K, V] {
	t := &TraitBy[K, V]{}

	t.Trait = *NewTrait(config, options...)

	return t
}

// PrepareRead handles cached entry.
func (c *TraitBy[K, V]) PrepareRead(ctx context.Context, cacheEntry *TraitEntryBy[K, V], found bool) (v V, err error) {
	if !found {
		if c.Log.logDebug != nil {
			c.Log.logDebug(ctx, "cache miss", "name", c.Config.Name)
		}

		if c.Stat != nil {
			c.Stat.Add(ctx, MetricMiss, 1, "name", c.Config.Name)
		}

		return v, ErrNotFound
	}

	now := ts(time.Now())

	if cacheEntry != nil && c.Config.EvictionStrategy != EvictMostExpired {
		switch c.Config.EvictionStrategy {
		case EvictLeastRecentlyUsed:
			atomic.StoreInt64(&cacheEntry.C, now)
		case EvictLeastFrequentlyUsed:
			atomic.AddInt64(&cacheEntry.C, 1)
		}
	}

	if cacheEntry.E != 0 && cacheEntry.E < now {
		if c.Log.logDebug != nil {
			c.Log.logDebug(ctx, "cache key expired", "name", c.Config.Name)
		}

		if c.Stat != nil {
			c.Stat.Add(ctx, MetricExpired, 1, "name", c.Config.Name)
		}

		return v, errExpiredOf[V]{entry: &TraitEntryOf[V]{
			V: cacheEntry.V,
			E: cacheEntry.E,
		}}
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricHit, 1, "name", c.Config.Name)
	}

	if c.Log.logDebug != nil {
		c.Log.logDebug(ctx, "cache hit",
			"name", c.Config.Name,
			"entry", cacheEntry,
		)
	}

	return cacheEntry.V, nil
}

// NotifyWritten collects logs and metrics.
func (c *TraitBy[K, V]) NotifyWritten(ctx context.Context, key K, value V, ttl time.Duration) {
	if c.Log.logDebug != nil {
		c.Log.logDebug(ctx, "wrote to cache",
			"name", c.Config.Name,
			"key", key,
			"value", value,
			"ttl", ttl,
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricWrite, 1, "name", c.Config.Name)
	}
}

// NotifyDeleted collects logs and metrics.
func (c *TraitBy[K, V]) NotifyDeleted(ctx context.Context, key K) {
	if c.Log.logDebug != nil {
		c.Log.logDebug(ctx, "deleted cache entry",
			"name", c.Config.Name,
			"key", key,
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricDelete, 1, "name", c.Config.Name)
	}
}

// TraitEntryBy is a cache entry.
type TraitEntryBy[K comparable, V any] struct {
	K K     `json:"key" description:"Typed key."`
	V V     `json:"val" description:"Typed value."`
	E int64 `json:"exp" description:"Expiration timestamp, ns."`
	C int64 `json:"-" description:"Usage count or last serve timestamp (ns)."`
}

var _ EntryBy[string, any] = TraitEntryBy[string, any]{}

// Key returns entry key.
func (e TraitEntryBy[K, V]) Key() K {
	return e.K
}

// Value returns entry value.
func (e TraitEntryBy[K, V]) Value() V {
	return e.V
}

// ExpireAt returns entry expiration time.
func (e TraitEntryBy[K, V]) ExpireAt() time.Time {
	return tsTime(e.E)
}
