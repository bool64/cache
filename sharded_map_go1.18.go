//go:build go1.18
// +build go1.18

package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

var (
	_ ReadWriterOf[any] = &shardedMapOf[any]{}
	_ Deleter           = &shardedMapOf[any]{}
	_ WalkerOf[any]     = &shardedMapOf[any]{}
)

// ShardedMapOf is an in-memory cache backend. Please use NewShardedMapOf to create it.
type ShardedMapOf[V any] struct {
	*shardedMapOf[V]
}

type hashedBucketOf[V any] struct {
	sync.RWMutex
	data map[uint64]*entryOf[V]
}

type shardedMapOf[V any] struct {
	hashedBuckets [shards]hashedBucketOf[V]

	*traitOf[V]
}

// NewShardedMapOf creates an instance of in-memory cache with optional configuration.
func NewShardedMapOf[V any](options ...func(cfg *Config)) *ShardedMapOf[V] {
	c := &shardedMapOf[V]{}
	C := &ShardedMapOf[V]{
		shardedMapOf: c,
	}

	for i := 0; i < shards; i++ {
		c.hashedBuckets[i].data = make(map[uint64]*entryOf[V])
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	c.traitOf = newTraitOf[V](c, cfg)

	runtime.SetFinalizer(C, func(m *ShardedMapOf[V]) {
		close(m.closed)
	})

	return C
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (c *shardedMapOf[V]) Load(key []byte) (val V, loaded bool) {
	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]
	b.RLock()
	defer b.RUnlock()

	cacheEntry, found := b.data[h]

	if !found || !bytes.Equal(cacheEntry.K, key) {
		return val, false
	}

	return cacheEntry.V, true
}

// Store sets the value for a key.
func (c *shardedMapOf[V]) Store(key []byte, val V) {
	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]
	b.Lock()
	defer b.Unlock()

	// Copy key to allow mutations of original argument.
	k := make([]byte, len(key))
	copy(k, key)

	b.data[h] = &entryOf[V]{V: val, K: k}
}

// Read gets value.
func (c *shardedMapOf[V]) Read(ctx context.Context, key []byte) (val V, _ error) {
	if SkipRead(ctx) {
		return val, ErrNotFound
	}

	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]
	b.RLock()
	cacheEntry, found := b.data[h]
	b.RUnlock()

	if !found || !bytes.Equal(cacheEntry.K, key) {
		cacheEntry = nil
		found = false
	}

	v, err := c.prepareRead(ctx, cacheEntry, found)
	if err != nil {
		return val, err
	}

	return v, nil
}

// Write sets value by the key.
func (c *shardedMapOf[V]) Write(ctx context.Context, k []byte, v V) error {
	h := xxhash.Sum64(k)
	b := &c.hashedBuckets[h%shards]
	b.Lock()
	defer b.Unlock()

	// ttl := c.config.TimeToLive
	ttl := TTL(ctx)
	if ttl == DefaultTTL {
		ttl = c.config.TimeToLive
	}

	if c.config.ExpirationJitter > 0 {
		ttl += time.Duration(float64(ttl) * c.config.ExpirationJitter * (rand.Float64() - 0.5)) // nolint:gosec
	}

	// Copy key to allow mutations of original argument.
	key := make([]byte, len(k))
	copy(key, k)

	b.data[h] = &entryOf[V]{V: v, K: key, E: time.Now().Add(ttl)}

	if c.logDebug != nil {
		c.logDebug(ctx, "wrote to cache",
			"name", c.config.Name,
			"key", string(key),
			"value", v,
			"ttl", ttl,
		)
	}

	if c.stat != nil {
		c.stat.Add(ctx, MetricWrite, 1, "name", c.config.Name)
	}

	return nil
}

// Delete removes value by the key.
//
// It fails with ErrNotFound if key does not exist.
func (c *shardedMapOf[V]) Delete(ctx context.Context, key []byte) error {
	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]

	b.Lock()
	defer b.Unlock()

	cachedEntry, found := b.data[h]
	if !found || !bytes.Equal(cachedEntry.K, key) {
		return ErrNotFound
	}

	delete(b.data, h)

	if c.logDebug != nil {
		c.logDebug(ctx, "deleted cache entry",
			"name", c.config.Name,
			"key", string(key),
		)
	}

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale cache.
func (c *shardedMapOf[V]) ExpireAll(ctx context.Context) {
	now := time.Now()
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.Lock()
		for h, v := range b.data {
			v.E = now
			b.data[h] = v
			cnt++
		}
		b.Unlock()
	}

	if c.logImportant != nil {
		c.logImportant(ctx, "expired all entries in cache",
			"name", c.config.Name,
			"elapsed", time.Since(now).String(),
			"count", cnt,
		)
	}
}

// DeleteAll erases all entries.
func (c *shardedMapOf[V]) DeleteAll(ctx context.Context) {
	now := time.Now()
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for h := range c.hashedBuckets[i].data {
			delete(b.data, h)
			cnt++
		}
		b.Unlock()
	}

	if c.logImportant != nil {
		c.logImportant(ctx, "deleted all entries in cache",
			"name", c.config.Name,
			"elapsed", time.Since(now).String(),
			"count", cnt,
		)
	}
}

func (c *shardedMapOf[V]) deleteExpiredBefore(expirationBoundary time.Time) {
	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for h, v := range b.data {
			if v.E.Before(expirationBoundary) {
				delete(b.data, h)
			}
		}
		b.Unlock()
	}

	if heapInUseOverflow(c.config) || countOverflow(c.config, c.Len) {
		c.evictOldest()
	}
}

// Len returns number of elements in cache.
func (c *shardedMapOf[V]) Len() int {
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
func (c *shardedMapOf[V]) Walk(walkFn func(e EntryOf[V]) error) (int, error) {
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

// WalkDumpRestorer is an adapter of a non-generic cache transfer interface.
func (c *ShardedMapOf[V]) WalkDumpRestorer() WalkDumpRestorer {
	cc := *c
	lc := shardedMapLegacyWalkerOf[V](cc)

	var w struct {
		Dumper
		Restorer
		Walker
	}

	w.Dumper = c
	w.Walker = &lc
	w.Restorer = c

	return w
}

type shardedMapLegacyWalkerOf[V any] ShardedMapOf[V]

// Walk walks cached entries.
func (c *shardedMapLegacyWalkerOf[V]) Walk(walkFn func(e Entry) error) (int, error) {
	n := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.RLock()
		for _, v := range c.hashedBuckets[i].data {
			b.RUnlock()

			e := entry{
				K: v.K,
				V: v.V,
				E: v.E,
			}

			err := walkFn(e)
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

// Dump saves cached entries and returns a number of processed entries.
//
// Dump uses encoding/gob to serialize cache entries, therefore it is necessary to
// register cached types in advance with cache.GobRegister.
func (c *ShardedMapOf[V]) Dump(w io.Writer) (int, error) {
	encoder := gob.NewEncoder(w)

	return c.Walk(func(e EntryOf[V]) error {
		return encoder.Encode(e)
	})
}

// Restore loads cached entries and returns number of processed entries.
//
// Restore uses encoding/gob to unserialize cache entries, therefore it is necessary to
// register cached types in advance with cache.GobRegister.
func (c *ShardedMapOf[V]) Restore(r io.Reader) (int, error) {
	var (
		decoder = gob.NewDecoder(r)
		n       = 0
	)

	for {
		var e entryOf[V]

		err := decoder.Decode(&e)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return n, err
		}

		h := xxhash.Sum64(e.K)
		b := &c.hashedBuckets[h%shards]

		b.Lock()
		b.data[h] = &e
		b.Unlock()

		n++
	}

	return n, nil
}
