package cache

import (
	"context"
	"encoding/gob"
	"errors"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	_ ReadWriter = &syncMap{}
	_ Deleter    = &syncMap{}
	_ Walker     = &syncMap{}
)

// SyncMap is an in-memory cache backend. Please use NewSyncMap to create it.
type SyncMap struct {
	*syncMap
}

type syncMap struct {
	*InvalidationIndex

	data sync.Map

	t *Trait
}

// NewSyncMap creates an instance of in-memory cache with optional configuration.
func NewSyncMap(options ...func(cfg *Config)) *SyncMap {
	c := &syncMap{}
	C := &SyncMap{
		syncMap: c,
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	evict := c.evictMostExpired

	if cfg.EvictionStrategy != EvictMostExpired {
		evict = c.evictLeastCounter
	}

	c.t = NewTrait(cfg, func(t *Trait) {
		t.DeleteExpired = c.deleteExpired
		t.Len = c.Len
		t.Evict = evict
	})

	c.InvalidationIndex = NewInvalidationIndex(c)

	runtime.SetFinalizer(C, func(m *SyncMap) {
		close(m.t.Closed)
	})

	return C
}

// Read gets value.
func (c *syncMap) Read(ctx context.Context, key []byte) (interface{}, error) {
	if SkipRead(ctx) {
		return nil, ErrNotFound
	}

	if cacheEntry, found := c.data.Load(string(key)); found {
		return c.t.PrepareRead(ctx, cacheEntry.(*TraitEntry), true)
	}

	return c.t.PrepareRead(ctx, nil, false)
}

// Write sets value by the key.
func (c *syncMap) Write(ctx context.Context, k []byte, v interface{}) error {
	// Copy key to allow mutations of original argument.
	key := make([]byte, len(k))
	copy(key, k)

	ttl, expireAt := c.t.expireAt(ctx)

	c.data.Store(string(k), &TraitEntry{V: v, K: key, E: expireAt})
	c.t.NotifyWritten(ctx, key, v, ttl)

	return nil
}

// Delete removes values by the key.
func (c *syncMap) Delete(ctx context.Context, key []byte) error {
	c.data.Delete(string(key))

	c.t.NotifyDeleted(ctx, key)

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale values.
func (c *syncMap) ExpireAll(ctx context.Context) {
	start := time.Now()
	startTS := ts(start)
	cnt := 0

	c.data.Range(func(key, value interface{}) bool {
		cacheEntry := value.(*TraitEntry) //nolint // Panic on type assertion failure is fine here.

		cacheEntry.E = startTS
		cnt++

		return true
	})

	c.t.NotifyExpiredAll(ctx, start, cnt)
}

// DeleteAll erases all entries.
func (c *syncMap) DeleteAll(ctx context.Context) {
	start := time.Now()
	cnt := 0

	c.data.Range(func(key, _ interface{}) bool {
		c.data.Delete(key)
		cnt++

		return true
	})

	c.t.NotifyDeletedAll(ctx, start, cnt)
}

func (c *syncMap) deleteExpired(before time.Time) {
	beforeTS := ts(before)

	c.data.Range(func(key, value interface{}) bool {
		cacheEntry := value.(*TraitEntry) //nolint // Panic on type assertion failure is fine here.
		if cacheEntry.E < beforeTS {
			c.data.Delete(key)
		}

		return true
	})
}

// Len returns number of elements including expired.
func (c *syncMap) Len() int {
	cnt := 0

	c.data.Range(func(key, value interface{}) bool {
		cnt++

		return true
	})

	return cnt
}

// Walk walks cached entries.
func (c *syncMap) Walk(walkFn func(e Entry) error) (int, error) {
	n := 0

	var lastErr error

	c.data.Range(func(key, value interface{}) bool {
		err := walkFn(value.(*TraitEntry))
		if err != nil {
			lastErr = err

			return false
		}

		n++

		return true
	})

	return n, lastErr
}

// Dump saves cached entries and returns a number of processed entries.
//
// Dump uses encoding/gob to serialize cache entries, therefore it is necessary to
// register cached types in advance with GobRegister.
func (c *SyncMap) Dump(w io.Writer) (int, error) {
	encoder := gob.NewEncoder(w)

	return c.Walk(func(e Entry) error {
		return encoder.Encode(e)
	})
}

// Restore loads cached entries and returns number of processed entries.
//
// Restore uses encoding/gob to unserialize cache entries, therefore it is necessary to
// register cached types in advance with GobRegister.
func (c *SyncMap) Restore(r io.Reader) (int, error) {
	var (
		decoder = gob.NewDecoder(r)
		e       TraitEntry
		n       = 0
	)

	for {
		err := decoder.Decode(&e)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return n, err
		}

		e := e

		c.data.Store(string(e.K), &e)

		n++
	}

	return n, nil
}

func (c *syncMap) evictMostExpired(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntry) int64 {
		return atomic.LoadInt64(&i.E)
	})
}

func (c *syncMap) evictLeastCounter(evictFraction float64) int {
	return c.evictLeast(evictFraction, func(i *TraitEntry) int64 {
		return atomic.LoadInt64(&i.C)
	})
}

func (c *syncMap) evictLeast(evictFraction float64, val func(i *TraitEntry) int64) int {
	type en struct {
		key string
		val int64
	}

	keysCnt := c.Len()
	entries := make([]en, 0, keysCnt)

	// Collect all keys and expirations.
	c.data.Range(func(key, value interface{}) bool {
		i := value.(*TraitEntry) //nolint // Panic on type assertion failure is fine here.
		entries = append(entries, en{val: val(i), key: string(i.K)})

		return true
	})

	// Sort entries to put most expired in head.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].val < entries[j].val
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	for i := 0; i < evictItems; i++ {
		c.data.Delete(entries[i].key)
	}

	return evictItems
}
