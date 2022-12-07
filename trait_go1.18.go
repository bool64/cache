//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// TraitOf is a parametrized shared trait, useful to implement ReadWriterOf.
type TraitOf[V any] struct {
	Trait
}

// NewTraitOf instantiates new TraitOf.
func NewTraitOf[V any](config Config, options ...func(t *Trait)) *TraitOf[V] {
	t := &TraitOf[V]{}

	t.Trait = *NewTrait(config, options...)

	return t
}

// PrepareRead handles cached entry.
func (c *TraitOf[V]) PrepareRead(ctx context.Context, cacheEntry *TraitEntryOf[V], found bool) (v V, err error) {
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

		return v, errExpiredOf[V]{entry: cacheEntry}
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
func (c *TraitOf[V]) NotifyWritten(ctx context.Context, key []byte, value V, ttl time.Duration) {
	if c.Log.logDebug != nil {
		c.Log.logDebug(ctx, "wrote to cache",
			"name", c.Config.Name,
			"key", string(key),
			"value", value,
			"ttl", ttl,
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricWrite, 1, "name", c.Config.Name)
	}
}

// TraitEntryOf is a cache entry.
type TraitEntryOf[V any] struct {
	K Key   `json:"key" description:"Cache entry key."`
	V V     `json:"val" description:"Cache entry value."`
	E int64 `json:"exp" description:"Expiration timestamp, ns."`
	C int64 `json:"-" description:"Usage count or last serve timestamp (ns)."`
}

var _ EntryOf[any] = TraitEntryOf[any]{}

// Key returns entry key.
func (e TraitEntryOf[V]) Key() []byte {
	return e.K
}

// Value returns entry value.
func (e TraitEntryOf[V]) Value() V {
	return e.V
}

// ExpireAt returns entry expiration time.
func (e TraitEntryOf[V]) ExpireAt() time.Time {
	return tsTime(e.E)
}

var _ ErrWithExpiredItemOf[any] = errExpiredOf[any]{}

type errExpiredOf[V any] struct {
	entry *TraitEntryOf[V]
}

func (e errExpiredOf[V]) Error() string {
	return ErrExpired.Error()
}

func (e errExpiredOf[V]) Value() V {
	return e.entry.V
}

func (e errExpiredOf[V]) ExpiredAt() time.Time {
	return tsTime(e.entry.E)
}

func (e errExpiredOf[V]) Is(err error) bool {
	return errors.Is(err, ErrExpired)
}
