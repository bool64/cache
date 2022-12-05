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
	"time"

	"github.com/cespare/xxhash/v2"
)

var (
	_ ReadWriter = &shardedMap{}
	_ Deleter    = &shardedMap{}
)

const shards = 128

type hashedBucket struct {
	sync.RWMutex
	data map[uint64]*TraitEntry
}

// ShardedMap is an in-memory cache backend. Please use NewShardedMap to create it.
type ShardedMap struct {
	*shardedMap
}

type shardedMap struct {
	hashedBuckets [shards]hashedBucket

	t *Trait
}

// NewShardedMap creates an instance of in-memory cache with optional configuration.
func NewShardedMap(options ...func(cfg *Config)) *ShardedMap {
	c := &shardedMap{}
	C := &ShardedMap{
		shardedMap: c,
	}

	for i := 0; i < shards; i++ {
		c.hashedBuckets[i].data = make(map[uint64]*TraitEntry)
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	c.t = NewTrait(cfg, func(t *Trait) {
		t.DeleteExpired = c.deleteExpired
		t.Len = c.Len
		t.EvictOldest = c.evictOldest
	})

	runtime.SetFinalizer(C, func(m *ShardedMap) {
		close(m.t.Closed)
	})

	return C
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (c *shardedMap) Load(key []byte) (interface{}, bool) {
	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]
	b.RLock()
	defer b.RUnlock()

	cacheEntry, found := b.data[h]

	if !found || !bytes.Equal(cacheEntry.K, key) {
		return nil, false
	}

	return cacheEntry.V, true
}

// Store sets the value for a key.
func (c *shardedMap) Store(key []byte, val interface{}) {
	h := xxhash.Sum64(key)
	b := &c.hashedBuckets[h%shards]
	b.Lock()
	defer b.Unlock()

	// Copy key to allow mutations of original argument.
	k := make([]byte, len(key))
	copy(k, key)

	b.data[h] = &TraitEntry{V: val, K: k}
}

// Read gets value.
func (c *shardedMap) Read(ctx context.Context, key []byte) (interface{}, error) {
	if SkipRead(ctx) {
		return nil, ErrNotFound
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

	return c.t.PrepareRead(ctx, cacheEntry, found)
}

// Write sets value by the key.
func (c *shardedMap) Write(ctx context.Context, k []byte, v interface{}) error {
	h := xxhash.Sum64(k)
	b := &c.hashedBuckets[h%shards]
	b.Lock()
	defer b.Unlock()

	// Copy key to allow mutations of original argument.
	key := make([]byte, len(k))
	copy(key, k)

	ttl, expireAt := c.t.expireAt(ctx)

	b.data[h] = &TraitEntry{V: v, K: key, E: expireAt}

	c.t.NotifyWritten(ctx, key, v, ttl)

	return nil
}

// Delete removes value by the key.
//
// It fails with ErrNotFound if key does not exist.
func (c *shardedMap) Delete(ctx context.Context, key []byte) error {
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
func (c *shardedMap) ExpireAll(ctx context.Context) {
	start := time.Now()
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]
		b.Lock()
		for h, v := range b.data {
			v.E = start
			b.data[h] = v
			cnt++
		}
		b.Unlock()
	}

	c.t.NotifyExpiredAll(ctx, start, cnt)
}

// DeleteAll erases all entries.
func (c *shardedMap) DeleteAll(ctx context.Context) {
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

	c.t.NotifyDeletedAll(ctx, now, cnt)
}

func (c *shardedMap) deleteExpired(before time.Time) {
	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.Lock()
		for h, v := range b.data {
			if v.E.Before(before) {
				delete(b.data, h)
			}
		}
		b.Unlock()
	}
}

// Len returns number of elements in cache.
func (c *shardedMap) Len() int {
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
func (c *shardedMap) Walk(walkFn func(e Entry) error) (int, error) {
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

// Dump saves cached entries and returns a number of processed entries.
//
// Dump uses encoding/gob to serialize cache entries, therefore it is necessary to
// register cached types in advance with cache.GobRegister.
func (c *ShardedMap) Dump(w io.Writer) (int, error) {
	encoder := gob.NewEncoder(w)

	return c.Walk(func(e Entry) error {
		return encoder.Encode(e)
	})
}

// Restore loads cached entries and returns number of processed entries.
//
// Restore uses encoding/gob to unserialize cache entries, therefore it is necessary to
// register cached types in advance with cache.GobRegister.
func (c *ShardedMap) Restore(r io.Reader) (int, error) {
	var (
		decoder = gob.NewDecoder(r)
		n       = 0
	)

	for {
		var e TraitEntry

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

type evictEntry struct {
	hash     uint64
	expireAt time.Time
}

func (c *shardedMap) evictOldest(evictFraction float64) int {
	cnt := 0

	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		cnt += len(b.data)
		b.RUnlock()
	}

	entries := make([]evictEntry, 0, cnt)

	// Collect all keys and expirations.
	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		for h, i := range b.data {
			entries = append(entries, evictEntry{hash: h, expireAt: i.E})
		}
		b.RUnlock()
	}

	// Sort entries to put most expired in head.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].expireAt.Before(entries[j].expireAt)
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
