//go:build go1.18
// +build go1.18

package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
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
	data map[uint64]*TraitEntryOf[V]
}

type shardedMapOf[V any] struct {
	*InvalidationIndex

	hashedBuckets [shards]hashedBucketOf[V]

	t *TraitOf[V]
}

// NewShardedMapOf creates an instance of in-memory cache with optional configuration.
func NewShardedMapOf[V any](options ...func(cfg *Config)) *ShardedMapOf[V] {
	c := &shardedMapOf[V]{}
	C := &ShardedMapOf[V]{
		shardedMapOf: c,
	}

	for i := 0; i < shards; i++ {
		c.hashedBuckets[i].data = make(map[uint64]*TraitEntryOf[V])
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	evict := c.evictMostExpired

	if cfg.EvictionStrategy != EvictMostExpired {
		evict = c.evictLeastCounter
	}

	c.t = NewTraitOf[V](cfg, func(t *Trait) {
		t.DeleteExpired = c.deleteExpired
		t.Len = c.Len
		t.Evict = evict
	})

	c.InvalidationIndex = NewInvalidationIndex(c)

	runtime.SetFinalizer(C, func(m *ShardedMapOf[V]) {
		close(m.t.Closed)
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

	b.data[h] = &TraitEntryOf[V]{V: val, K: k}
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

	v, err := c.t.PrepareRead(ctx, cacheEntry, found)
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

	// Copy key to allow mutations of original argument.
	key := make([]byte, len(k))
	copy(key, k)

	ttl, expireAt := c.t.expireAt(ctx)

	b.data[h] = &TraitEntryOf[V]{V: v, K: key, E: expireAt}

	c.t.NotifyWritten(ctx, key, v, ttl)

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

	c.t.NotifyDeleted(ctx, key)

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale cache.
func (c *shardedMapOf[V]) ExpireAll(ctx context.Context) {
	start := time.Now()
	startTS := ts(start)
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.Lock()
		for h, v := range b.data {
			v.E = startTS
			b.data[h] = v
			cnt++
		}
		b.Unlock()
	}

	c.t.NotifyExpiredAll(ctx, start, cnt)
}

// DeleteAll erases all entries.
func (c *shardedMapOf[V]) DeleteAll(ctx context.Context) {
	start := time.Now()
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

	c.t.NotifyDeletedAll(ctx, start, cnt)
}

func (c *shardedMapOf[V]) deleteExpired(before time.Time) {
	beforeTS := ts(before)

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for h, v := range b.data {
			if v.E < beforeTS {
				delete(b.data, h)
			}
		}
		b.Unlock()
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

			e := TraitEntry{
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
		var e TraitEntryOf[V]

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

func (c *shardedMapOf[V]) evictMostExpired(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryOf[V]) int64 {
		return atomic.LoadInt64(&i.E)
	})
}

func (c *shardedMapOf[V]) evictLeastCounter(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntryOf[V]) int64 {
		return atomic.LoadInt64(&i.C)
	})
}

//nolint:dupl // Hard to deduplicate due to generic constraints.
func (c *shardedMapOf[V]) evictLeast(evictFraction float64, val func(i *TraitEntryOf[V]) int64) int {
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		cnt += len(b.data)
		b.RUnlock()
	}

	entries := make([]evictLeastEntry, 0, cnt)

	// Collect all keys and expirations.
	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		for h, i := range b.data {
			entries = append(entries, evictLeastEntry{hash: h, val: val(i)})
		}
		b.RUnlock()
	}

	// Sort entries to put most expired in head.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].val < entries[j].val
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	for i := 0; i < evictItems; i++ {
		h := entries[i].hash
		b := &c.hashedBuckets[h%shards]

		b.Lock()
		delete(b.data, h)
		b.Unlock()
	}

	return evictItems
}
