//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"time"

	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
)

type traitOf[V any] struct {
	closed chan struct{}

	config Config
	log    ctxd.Logger
	stat   stats.Tracker
}

func newTraitOf[V any](b backend, config Config) *traitOf[V] {
	if config.DeleteExpiredAfter == 0 {
		config.DeleteExpiredAfter = 24 * time.Hour
	}

	if config.DeleteExpiredJobInterval == 0 {
		config.DeleteExpiredJobInterval = time.Hour
	}

	if config.ItemsCountReportInterval == 0 {
		config.ItemsCountReportInterval = time.Minute
	}

	if config.ExpirationJitter == 0 {
		config.ExpirationJitter = 0.1
	}

	if config.TimeToLive == 0 {
		config.TimeToLive = 5 * time.Minute
	}

	t := &traitOf[V]{
		config: config,
		stat:   config.Stats,
		log:    config.Logger,
		closed: make(chan struct{}),
	}

	if config.Stats != nil {
		go config.reportItemsCount(b, t.closed)
	}

	go config.janitor(b, t.closed)

	return t
}

func (c *traitOf[V]) prepareRead(ctx context.Context, cacheEntry *entryOf[V], found bool) (v V, err error) {
	if !found {
		if c.log != nil {
			c.log.Debug(ctx, "cache miss", "name", c.config.Name)
		}

		if c.stat != nil {
			c.stat.Add(ctx, MetricMiss, 1, "name", c.config.Name)
		}

		return v, ErrNotFound
	}

	if cacheEntry.E.Before(time.Now()) {
		if c.log != nil {
			c.config.Logger.Debug(ctx, "cache key expired", "name", c.config.Name)
		}

		if c.stat != nil {
			c.stat.Add(ctx, MetricExpired, 1, "name", c.config.Name)
		}

		return v, errExpiredOf[V]{entry: cacheEntry}
	}

	if c.stat != nil {
		c.stat.Add(ctx, MetricHit, 1, "name", c.config.Name)
	}

	if c.log != nil {
		c.log.Debug(ctx, "cache hit",
			"name", c.config.Name,
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
