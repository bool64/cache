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
	_ ReadWriterBy[string, any] = &shardedMapBy[string, any]{}
	_ WalkerBy[string, any]     = &shardedMapBy[string, any]{}
)

// ShardedMapBy is an in-memory cache backend with typed keys. Please use NewShardedMapBy to create it.
type ShardedMapBy[K comparable, V any] struct {
	*shardedMapBy[K, V]
}

type keyedBucketBy[K comparable, V any] struct {
	sync.RWMutex
	data map[K]*TraitEntryBy[K, V]
}

type shardedMapBy[K comparable, V any] struct {
	shard func(K) uint64

	hashedBuckets [shards]keyedBucketBy[K, V]

	t *TraitBy[K, V]
}

// NewShardedMapBy creates an instance of in-memory cache with typed keys and optional configuration.
func NewShardedMapBy[K comparable, V any](options ...func(cfg *ConfigBy[K])) *ShardedMapBy[K, V] {
	c := &shardedMapBy[K, V]{}
	C := &ShardedMapBy[K, V]{
		shardedMapBy: c,
	}

	for i := 0; i < shards; i++ {
		c.hashedBuckets[i].data = make(map[K]*TraitEntryBy[K, V])
	}

	cfg := ConfigBy[K]{}
	for _, option := range options {
		option(&cfg)
	}

	c.shard = resolveShardFunc(cfg)

	evict := c.evictMostExpired

	if cfg.EvictionStrategy != EvictMostExpired {
		evict = c.evictLeastCounter
	}

	c.t = NewTraitBy[K, V](cfg.Config, func(t *Trait) {
		t.DeleteExpired = c.deleteExpired
		t.Len = c.Len
		t.Evict = evict
	})

	runtime.SetFinalizer(C, func(m *ShardedMapBy[K, V]) {
		close(m.t.Closed)
	})

	return C
}

// Load returns the value stored in the map for a key, or a zero value if no value is present.
func (c *shardedMapBy[K, V]) Load(key K) (val V, loaded bool) {
	v, err := c.Read(bgCtx, key)
	if err != nil {
		return val, false
	}

	return v, true
}

// Store sets the value for a key.
func (c *shardedMapBy[K, V]) Store(key K, val V) {
	err := c.Write(bgCtx, key, val)
	if err != nil && c.t.Log.logError != nil {
		c.t.Log.logError(bgCtx, "failed to store cache entry",
			"error", err,
			"key", key,
			"name", c.t.Config.Name)
	}
}

// Read gets value.
func (c *shardedMapBy[K, V]) Read(ctx context.Context, key K) (val V, _ error) {
	if SkipRead(ctx) {
		return val, ErrNotFound
	}

	h := c.shard(key)
	b := &c.hashedBuckets[h%shards]
	b.RLock()
	cacheEntry, found := b.data[key]
	b.RUnlock()

	v, err := c.t.PrepareRead(ctx, cacheEntry, found)
	if err != nil {
		return val, err
	}

	return v, nil
}

// Write sets value by the key.
func (c *shardedMapBy[K, V]) Write(ctx context.Context, key K, v V) error {
	h := c.shard(key)
	b := &c.hashedBuckets[h%shards]
	b.Lock()
	defer b.Unlock()

	ttl, expireAt := c.t.expireAt(ctx)

	b.data[key] = &TraitEntryBy[K, V]{V: v, K: key, E: expireAt}

	c.t.NotifyWritten(ctx, key, v, ttl)

	return nil
}

// Delete removes value by the key.
func (c *shardedMapBy[K, V]) Delete(ctx context.Context, key K) error {
	h := c.shard(key)
	b := &c.hashedBuckets[h%shards]

	b.Lock()
	defer b.Unlock()

	if _, found := b.data[key]; !found {
		return ErrNotFound
	}

	delete(b.data, key)

	c.t.NotifyDeleted(ctx, key)

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale cache.
func (c *shardedMapBy[K, V]) ExpireAll(ctx context.Context) {
	start := time.Now()
	startTS := ts(start)
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.Lock()
		for k, v := range b.data {
			v.E = startTS
			b.data[k] = v

			cnt++
		}
		b.Unlock()
	}

	c.t.NotifyExpiredAll(ctx, start, cnt)
}

// DeleteAll erases all entries.
func (c *shardedMapBy[K, V]) DeleteAll(ctx context.Context) {
	start := time.Now()
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for k := range c.hashedBuckets[i].data {
			delete(b.data, k)

			cnt++
		}
		b.Unlock()
	}

	c.t.NotifyDeletedAll(ctx, start, cnt)
}

func (c *shardedMapBy[K, V]) deleteExpired(before time.Time) {
	beforeTS := ts(before)

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for k, v := range b.data {
			if v.E < beforeTS {
				delete(b.data, k)
			}
		}
		b.Unlock()
	}
}

// Len returns number of elements in cache.
func (c *shardedMapBy[K, V]) Len() int {
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		cnt += len(b.data)
		b.RUnlock()
	}

	return cnt
}

// Walk walks cached entries.
func (c *shardedMapBy[K, V]) Walk(walkFn func(e EntryBy[K, V]) error) (int, error) {
	n := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.RLock()
		for _, v := range c.hashedBuckets[i].data {
			b.RUnlock()

			err := walkFn(v)
			if err != nil {
				return n, err
			}

			n++

			b.RLock()
		}
		b.RUnlock()
	}

	return n, nil
}

func (c *shardedMapBy[K, V]) evictMostExpired(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryBy[K, V]) int64 {
		return atomic.LoadInt64(&i.E)
	})
}

func (c *shardedMapBy[K, V]) evictLeastCounter(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryBy[K, V]) int64 {
		return atomic.LoadInt64(&i.C)
	})
}

func (c *shardedMapBy[K, V]) evictLeast(evictFraction float64, val func(i *TraitEntryBy[K, V]) int64) int {
	type evictLeastEntryBy struct {
		key   K
		shard int
		val   int64
	}

	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		cnt += len(b.data)
		b.RUnlock()
	}

	entries := make([]evictLeastEntryBy, 0, cnt)

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		for k, entry := range b.data {
			entries = append(entries, evictLeastEntryBy{key: k, shard: i, val: val(entry)})
		}
		b.RUnlock()
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].val < entries[j].val
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	for i := 0; i < evictItems; i++ {
		e := entries[i]
		b := &c.hashedBuckets[e.shard]

		b.Lock()
		delete(b.data, e.key)
		b.Unlock()
	}

	return evictItems
}
