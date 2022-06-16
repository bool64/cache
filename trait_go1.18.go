//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"time"
)

// TraitOf is a parametrized shared trait, useful to implement ReadWriterOff.
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

	if cacheEntry.E.Before(time.Now()) {
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
	K Key       `json:"key"`
	V V         `json:"val"`
	E time.Time `json:"exp"`
}

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
	return e.E
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
	return e.entry.E
}

func (e errExpiredOf[V]) Is(err error) bool {
	return err == ErrExpired // nolint:errorlint,goerr113  // Target sentinel error is not wrapped.
}
