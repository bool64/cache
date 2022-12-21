package cache

import (
	"context"
	"errors"
	"math/rand"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
)

func (c *Trait) reportItemsCount() {
	for {
		interval := c.Config.ItemsCountReportInterval

		select {
		case <-time.After(interval):
			count := c.Len()

			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "cache items count",
					"name", c.Config.Name,
					"count", c.Len(),
				)
			}

			if c.Stat != nil {
				c.Stat.Set(context.Background(), MetricItems, float64(count), "name", c.Config.Name)
			}
		case <-c.Closed:
			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "closing cache items counter goroutine",
					"name", c.Config.Name)
			}

			if c.Stat != nil {
				c.Stat.Set(context.Background(), MetricItems, float64(c.Len()), "name", c.Config.Name)
			}

			return
		}
	}
}

func (c *Trait) janitor() {
	for {
		interval := c.Config.DeleteExpiredJobInterval

		select {
		case <-time.After(interval):
			c.invokeCleanup()

		case <-c.Closed:
			if c.Log.logDebug != nil {
				c.Log.logDebug(context.Background(), "closing cache janitor",
					"name", c.Config.Name)
			}

			return
		}
	}
}

func (c *Trait) invokeCleanup() {
	// Delete expired job is skipped for UnlimitedTTL with a proof of no expirations were set before.
	// This is an optimization to avoid full scan and make eviction checks/cleanups cheap.
	if c.DeleteExpired != nil && (c.Config.TimeToLive != UnlimitedTTL || atomic.LoadInt64(&c.expirationsSet) > 0) {
		expirationBoundary := time.Now().Add(-c.Config.DeleteExpiredAfter)
		c.DeleteExpired(expirationBoundary)
	}

	if c.Evict == nil {
		return
	}

	ho := c.heapInUseOverflow()
	currentCnt, co := c.countOverflow()
	so := c.sysOverflow()

	if ho || so || co {
		frac := c.Config.EvictFraction
		if frac == 0 {
			frac = 0.1
		}

		// For count overflow we're updating fraction to reach the level below CountSoftLimit.
		// This might be a more aggressive eviction in case when cache growth exceeds eviction rate (e.g.
		// if evicting EvictFraction would still leave the count above CountSoftLimit).
		if co {
			targetCnt := float64(c.Config.CountSoftLimit) * (1 - frac)
			frac = 1 - targetCnt/float64(currentCnt)
		}

		cnt := c.Evict(frac)

		if so {
			debug.FreeOSMemory()
		}

		if c.Stat != nil {
			c.Stat.Add(context.Background(), MetricEvict, float64(cnt), "name", c.Config.Name)
		}
	}
}

func (c *Trait) heapInUseOverflow() bool {
	if c.Config.HeapInUseSoftLimit == 0 {
		return false
	}

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	return m.HeapInuse > c.Config.HeapInUseSoftLimit
}

func (c *Trait) sysOverflow() bool {
	if c.Config.SysMemSoftLimit == 0 {
		return false
	}

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	return m.Sys > c.Config.SysMemSoftLimit
}

func (c *Trait) countOverflow() (int, bool) {
	if c.Config.CountSoftLimit == 0 || c.Len == nil {
		return 0, false
	}

	cnt := c.Len()

	return cnt, cnt > int(c.Config.CountSoftLimit)
}

// Trait is a shared trait, useful to implement ReadWriter.
type Trait struct {
	Closed chan struct{}

	DeleteExpired func(before time.Time)
	Len           func() int
	Evict         func(fraction float64) int

	Config Config
	Stat   StatsTracker
	Log    logTrait

	expirationsSet int64
}

// NewTrait instantiates new Trait.
func NewTrait(config Config, options ...func(t *Trait)) *Trait {
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

	t := &Trait{
		Config: config,
		Stat:   config.Stats,
		Closed: make(chan struct{}),
	}
	t.Log.setup(config.Logger)

	for _, o := range options {
		o(t)
	}

	if config.Stats != nil && t.Len != nil {
		go t.reportItemsCount()
	}

	if t.DeleteExpired != nil || t.Evict != nil {
		go t.janitor()
	}

	return t
}

// PrepareRead handles cached entry.
func (c *Trait) PrepareRead(ctx context.Context, cacheEntry *TraitEntry, found bool) (interface{}, error) {
	if !found {
		if c.Log.logDebug != nil {
			c.Log.logDebug(ctx, "cache miss", "name", c.Config.Name)
		}

		if c.Stat != nil {
			c.Stat.Add(ctx, MetricMiss, 1, "name", c.Config.Name)
		}

		return nil, ErrNotFound
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

func (c *Trait) expireAt(ctx context.Context) (time.Duration, int64) {
	if ttl := c.TTL(ctx); ttl != 0 {
		return ttl, ts(time.Now().Add(ttl))
	}

	return 0, 0
}

// TTL calculates time to live for a new entry.
func (c *Trait) TTL(ctx context.Context) time.Duration {
	ttl := TTL(ctx)
	if ttl == DefaultTTL {
		if c.Config.TimeToLive == UnlimitedTTL {
			return 0
		}

		ttl = c.Config.TimeToLive
	}

	if c.Config.ExpirationJitter > 0 {
		ttl += time.Duration(float64(ttl) * c.Config.ExpirationJitter * (rand.Float64() - 0.5)) //nolint:gosec
	}

	if c.Config.TimeToLive == UnlimitedTTL && ttl != 0 {
		atomic.AddInt64(&c.expirationsSet, 1)
	}

	return ttl
}

// NotifyWritten collects logs and metrics.
func (c *Trait) NotifyWritten(ctx context.Context, key []byte, value interface{}, ttl time.Duration) {
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

// NotifyDeleted collects logs and metrics.
func (c *Trait) NotifyDeleted(ctx context.Context, key []byte) {
	if c.Log.logDebug != nil {
		c.Log.logDebug(ctx, "deleted cache entry",
			"name", c.Config.Name,
			"key", string(key),
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricDelete, 1, "name", c.Config.Name)
	}
}

// NotifyExpiredAll collects logs and metrics.
func (c *Trait) NotifyExpiredAll(ctx context.Context, start time.Time, cnt int) {
	if c.Log.logImportant != nil {
		c.Log.logImportant(ctx, "expired all entries in cache",
			"name", c.Config.Name,
			"elapsed", time.Since(start).String(),
			"count", cnt,
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricExpired, float64(cnt), "name", c.Config.Name)
	}
}

// NotifyDeletedAll collects logs and metrics.
func (c *Trait) NotifyDeletedAll(ctx context.Context, start time.Time, cnt int) {
	if c.Log.logImportant != nil {
		c.Log.logImportant(ctx, "deleted all entries in cache",
			"name", c.Config.Name,
			"elapsed", time.Since(start).String(),
			"count", cnt,
		)
	}

	if c.Stat != nil {
		c.Stat.Add(ctx, MetricDelete, float64(cnt), "name", c.Config.Name)
	}
}

// Key os a key of cached entry.
type Key []byte

// MarshalText renders bytes as text.
func (ks Key) MarshalText() ([]byte, error) {
	return ks, nil
}

// TraitEntry is a cache entry.
type TraitEntry struct {
	K Key         `json:"key" description:"Key."`
	V interface{} `json:"val" description:"Value."`
	E int64       `json:"exp" description:"Expiration timestamp (ns)."`
	C int64       `json:"-" description:"Usage count or last serve timestamp (ns)."`
}

var _ Entry = TraitEntry{}

// Key returns entry key.
func (e TraitEntry) Key() []byte {
	return e.K
}

// Value returns entry value.
func (e TraitEntry) Value() interface{} {
	return e.V
}

// ExpireAt returns entry expiration time.
func (e TraitEntry) ExpireAt() time.Time {
	return tsTime(e.E)
}

type errExpired struct {
	entry *TraitEntry
}

func (e errExpired) Error() string {
	return ErrExpired.Error()
}

func (e errExpired) Value() interface{} {
	return e.entry.V
}

func (e errExpired) ExpiredAt() time.Time {
	return tsTime(e.entry.E)
}

func (e errExpired) Is(err error) bool {
	return errors.Is(err, ErrExpired)
}

func ts(t time.Time) int64 {
	return t.UnixNano()
}

func tsTime(ns int64) time.Time {
	return time.Unix(ns/1e9, ns%1e9)
}
