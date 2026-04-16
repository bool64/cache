//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	_ ReadWriterBy[string, any] = &syncMapBy[string, any]{}
	_ WalkerBy[string, any]     = &syncMapBy[string, any]{}
)

// SyncMapBy is an in-memory cache backend with typed keys. Please use NewSyncMapBy to create it.
type SyncMapBy[K comparable, V any] struct {
	*syncMapBy[K, V]
}

type syncMapBy[K comparable, V any] struct {
	data sync.Map

	t *TraitBy[K, V]
}

// NewSyncMapBy creates an instance of in-memory cache with typed keys and optional configuration.
func NewSyncMapBy[K comparable, V any](options ...func(cfg *Config)) *SyncMapBy[K, V] {
	c := &syncMapBy[K, V]{}
	C := &SyncMapBy[K, V]{
		syncMapBy: c,
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	evict := c.evictMostExpired

	if cfg.EvictionStrategy != EvictMostExpired {
		evict = c.evictLeastCounter
	}

	c.t = NewTraitBy[K, V](cfg, func(t *Trait) {
		t.DeleteExpired = c.deleteExpired
		t.Len = c.Len
		t.Evict = evict
	})

	runtime.SetFinalizer(C, func(m *SyncMapBy[K, V]) {
		close(m.t.Closed)
	})

	return C
}

// Load returns the value stored in the map for a key, or a zero value if no value is present.
func (c *syncMapBy[K, V]) Load(key K) (val V, loaded bool) {
	v, err := c.Read(bgCtx, key)
	if err != nil {
		return val, false
	}

	return v, true
}

// Store sets the value for a key.
func (c *syncMapBy[K, V]) Store(key K, val V) {
	err := c.Write(bgCtx, key, val)
	if err != nil && c.t.Log.logError != nil {
		c.t.Log.logError(bgCtx, "failed to store cache entry",
			"error", err,
			"key", key,
			"name", c.t.Config.Name)
	}
}

// Read gets value.
func (c *syncMapBy[K, V]) Read(ctx context.Context, key K) (val V, _ error) {
	if SkipRead(ctx) {
		return val, ErrNotFound
	}

	if cacheEntry, found := c.data.Load(key); found {
		return c.t.PrepareRead(ctx, cacheEntry.(*TraitEntryBy[K, V]), true)
	}

	return c.t.PrepareRead(ctx, nil, false)
}

// Write sets value by the key.
func (c *syncMapBy[K, V]) Write(ctx context.Context, key K, v V) error {
	ttl, expireAt := c.t.expireAt(ctx)

	c.data.Store(key, &TraitEntryBy[K, V]{V: v, K: key, E: expireAt})
	c.t.NotifyWritten(ctx, key, v, ttl)

	return nil
}

// Delete removes values by the key.
func (c *syncMapBy[K, V]) Delete(ctx context.Context, key K) error {
	_, found := c.data.Load(key)
	if !found {
		return ErrNotFound
	}

	c.data.Delete(key)

	c.t.NotifyDeleted(ctx, key)

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale values.
func (c *syncMapBy[K, V]) ExpireAll(ctx context.Context) {
	start := time.Now()
	startTS := ts(start)
	cnt := 0

	c.data.Range(func(_, value interface{}) bool {
		cacheEntry := value.(*TraitEntryBy[K, V])

		cacheEntry.E = startTS

		cnt++

		return true
	})

	c.t.NotifyExpiredAll(ctx, start, cnt)
}

// DeleteAll erases all entries.
func (c *syncMapBy[K, V]) DeleteAll(ctx context.Context) {
	start := time.Now()
	cnt := 0

	c.data.Range(func(key, _ interface{}) bool {
		c.data.Delete(key)

		cnt++

		return true
	})

	c.t.NotifyDeletedAll(ctx, start, cnt)
}

func (c *syncMapBy[K, V]) deleteExpired(before time.Time) {
	beforeTS := ts(before)

	c.data.Range(func(key, value interface{}) bool {
		cacheEntry := value.(*TraitEntryBy[K, V])
		if cacheEntry.E < beforeTS {
			c.data.Delete(key)
		}

		return true
	})
}

// Len returns number of elements including expired.
func (c *syncMapBy[K, V]) Len() int {
	cnt := 0

	c.data.Range(func(_, _ interface{}) bool {
		cnt++

		return true
	})

	return cnt
}

// Walk walks cached entries.
func (c *syncMapBy[K, V]) Walk(walkFn func(e EntryBy[K, V]) error) (int, error) {
	n := 0

	var lastErr error

	c.data.Range(func(_, value interface{}) bool {
		err := walkFn(value.(*TraitEntryBy[K, V]))
		if err != nil {
			lastErr = err

			return false
		}

		n++

		return true
	})

	return n, lastErr
}

func (c *syncMapBy[K, V]) evictMostExpired(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryBy[K, V]) int64 {
		return atomic.LoadInt64(&i.E)
	})
}

func (c *syncMapBy[K, V]) evictLeastCounter(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryBy[K, V]) int64 {
		return atomic.LoadInt64(&i.C)
	})
}

func (c *syncMapBy[K, V]) evictLeast(evictFraction float64, val func(i *TraitEntryBy[K, V]) int64) int {
	type en struct {
		key K
		val int64
	}

	keysCnt := c.Len()
	entries := make([]en, 0, keysCnt)

	c.data.Range(func(key, value interface{}) bool {
		i := value.(*TraitEntryBy[K, V])
		entries = append(entries, en{val: val(i), key: key.(K)})

		return true
	})

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].val < entries[j].val
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	for i := 0; i < evictItems; i++ {
		c.data.Delete(entries[i].key)
	}

	return evictItems
}
