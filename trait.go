package cache

import (
	"context"
	"time"
)

func (c *trait) reportItemsCount(b backend, closed chan struct{}) {
	for {
		interval := c.Config.ItemsCountReportInterval

		select {
		case <-time.After(interval):
			count := b.Len()

			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "cache items count",
					"name", c.Config.Name,
					"count", b.Len(),
				)
			}

			if c.Stat != nil {
				c.Stat.Set(context.Background(), MetricItems, float64(count), "name", c.Config.Name)
			}
		case <-closed:
			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "closing cache items counter goroutine",
					"name", c.Config.Name)
			}

			if c.Stat != nil {
				c.Stat.Set(context.Background(), MetricItems, float64(b.Len()), "name", c.Config.Name)
			}

			return
		}
	}
}

func (c *trait) janitor(b backend, closed chan struct{}) {
	for {
		interval := c.Config.DeleteExpiredJobInterval

		select {
		case <-time.After(interval):
			expirationBoundary := time.Now().Add(-c.Config.DeleteExpiredAfter)
			b.deleteExpiredBefore(expirationBoundary)
		case <-closed:
			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "closing cache janitor",
					"name", c.Config.Name)
			}

			return
		}
	}
}

type trait struct {
	Closed chan struct{}

	Config Config
	Stat   StatsTracker
	Log    logTrait
}

type backend interface {
	Len() int
	deleteExpiredBefore(t time.Time)
}

func newTrait(b backend, config Config) *trait {
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

	t := &trait{
		Config: config,
		Stat:   config.Stats,
		Closed: make(chan struct{}),
	}
	t.Log.setup(config.Logger)

	if config.Stats != nil {
		go t.reportItemsCount(b, t.Closed)
	}

	go t.janitor(b, t.Closed)

	return t
}

func (c *trait) prepareRead(ctx context.Context, cacheEntry *entry, found bool) (interface{}, error) {
	if !found {
		if c.Log.logDebug != nil {
			c.Log.logDebug(ctx, "cache miss", "name", c.Config.Name)
		}

		if c.Stat != nil {
			c.Stat.Add(ctx, MetricMiss, 1, "name", c.Config.Name)
		}

		return nil, ErrNotFound
	}

	if cacheEntry.E.Before(time.Now()) {
		if c.Log.logDebug != nil {
			c.Log.logDebug(ctx, "cache key expired", "name", c.Config.Name)
		}

		if c.Stat != nil {
			c.Stat.Add(ctx, MetricExpired, 1, "name", c.Config.Name)
		}

		return nil, errExpired{entry: cacheEntry}
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

type keyString []byte

func (ks keyString) MarshalText() ([]byte, error) {
	return ks, nil
}

// entry is a cache entry.
type entry struct {
	K keyString   `json:"key"`
	V interface{} `json:"val"`
	E time.Time   `json:"exp"`
}

func (e entry) Key() []byte {
	return e.K
}

func (e entry) Value() interface{} {
	return e.V
}

func (e entry) ExpireAt() time.Time {
	return e.E
}

type errExpired struct {
	entry *entry
}

func (e errExpired) Error() string {
	return ErrExpired.Error()
}

func (e errExpired) Value() interface{} {
	return e.entry.V
}

func (e errExpired) ExpiredAt() time.Time {
	return e.entry.E
}

func (e errExpired) Is(err error) bool {
	return err == ErrExpired // nolint:errorlint,goerr113  // Target sentinel error is not wrapped.
}
