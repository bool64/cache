package cache

import (
	"context"
	"encoding/gob"
	"errors"
	"io"
	"math/rand"
	"runtime"
	"sort"
	"time"

	"github.com/puzpuzpuz/xsync"
)

var (
	_ ReadWriter = &xsyncMap{}
	_ Deleter    = &xsyncMap{}
)

// XsyncMap is an in-memory cache backend. Please use NewXsyncMap to create it.
type XsyncMap struct {
	*xsyncMap
}

type xsyncMap struct {
	data *xsync.Map

	*trait
}

// NewXsyncMap creates an instance of in-memory cache with optional configuration.
func NewXsyncMap(options ...func(cfg *Config)) *XsyncMap {
	c := &xsyncMap{}
	c.data = xsync.NewMap()
	C := &XsyncMap{
		xsyncMap: c,
	}

	cfg := Config{}
	for _, option := range options {
		option(&cfg)
	}

	c.trait = newTrait(c, cfg)

	runtime.SetFinalizer(C, func(m *XsyncMap) {
		close(m.closed)
	})

	return C
}

// Read gets value.
func (c *xsyncMap) Read(ctx context.Context, key []byte) (interface{}, error) {
	if SkipRead(ctx) {
		return nil, ErrNotFound
	}

	cacheEntry, found := c.data.Load(string(key))

	return c.prepareRead(ctx, cacheEntry, found)
}

// Write sets value by the key.
func (c *xsyncMap) Write(ctx context.Context, k []byte, v interface{}) error {
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

	c.data.Store(string(k), &entry{V: v, K: key, E: time.Now().Add(ttl)})

	if c.stat != nil {
		c.stat.Add(ctx, MetricWrite, 1, "name", c.config.Name)
	}

	if c.log != nil {
		c.log.Debug(ctx, "wrote to cache",
			"name", c.config.Name,
			"key", string(key),
			"value", v,
			"ttl", ttl,
		)
	}

	return nil
}

// Delete removes values by the key.
func (c *xsyncMap) Delete(ctx context.Context, key []byte) error {
	c.data.Delete(string(key))

	if c.log != nil {
		c.log.Debug(ctx, "deleted cache entry",
			"name", c.config.Name,
			"key", string(key),
		)
	}

	return nil
}

// ExpireAll marks all entries as expired, they can still serve stale values.
func (c *xsyncMap) ExpireAll(ctx context.Context) {
	now := time.Now()
	cnt := 0

	c.data.Range(func(key string, value interface{}) bool {
		cacheEntry := value.(*entry) // nolint // Panic on type assertion failure is fine here.

		cacheEntry.E = now
		cnt++

		return true
	})

	if c.log != nil {
		c.log.Important(ctx, "expired all entries in cache",
			"name", c.config.Name,
			"elapsed", time.Since(now).String(),
			"count", cnt,
		)
	}
}

// DeleteAll erases all entries.
func (c *xsyncMap) DeleteAll(ctx context.Context) {
	now := time.Now()
	cnt := 0

	c.data.Range(func(key string, _ interface{}) bool {
		c.data.Delete(key)
		cnt++

		return true
	})

	if c.log != nil {
		c.log.Important(ctx, "deleted all entries in cache",
			"name", c.config.Name,
			"elapsed", time.Since(now).String(),
			"count", cnt,
		)
	}
}

func (c *xsyncMap) deleteExpiredBefore(expirationBoundary time.Time) {
	c.data.Range(func(key string, value interface{}) bool {
		cacheEntry := value.(*entry) // nolint // Panic on type assertion failure is fine here.
		if cacheEntry.E.Before(expirationBoundary) {
			c.data.Delete(key)
		}

		return true
	})

	if c.heapInUseOverflow() || c.countOverflow() {
		c.evictOldest()
	}
}

// Len returns number of elements including expired.
func (c *xsyncMap) Len() int {
	cnt := 0

	c.data.Range(func(key string, value interface{}) bool {
		cnt++

		return true
	})

	return cnt
}

// Walk walks cached entries.
func (c *xsyncMap) Walk(walkFn func(e Entry) error) (int, error) {
	n := 0

	var lastErr error

	c.data.Range(func(key string, value interface{}) bool {
		err := walkFn(value.(*entry))
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
func (c *XsyncMap) Dump(w io.Writer) (int, error) {
	encoder := gob.NewEncoder(w)

	return c.Walk(func(e Entry) error {
		return encoder.Encode(e)
	})
}

// Restore loads cached entries and returns number of processed entries.
//
// Restore uses encoding/gob to unserialize cache entries, therefore it is necessary to
// register cached types in advance with GobRegister.
func (c *XsyncMap) Restore(r io.Reader) (int, error) {
	var (
		decoder = gob.NewDecoder(r)
		e       entry
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

func (c *xsyncMap) evictOldest() {
	evictFraction := c.config.EvictFraction
	if evictFraction == 0 {
		evictFraction = 0.1
	}

	type en struct {
		key      string
		expireAt time.Time
	}

	keysCnt := c.Len()
	entries := make([]en, 0, keysCnt)

	// Collect all keys and expirations.
	c.data.Range(func(key string, value interface{}) bool {
		i := value.(*entry) // nolint // Panic on type assertion failure is fine here.
		entries = append(entries, en{expireAt: i.E, key: string(i.K)})

		return true
	})

	// Sort entries to put most expired in head.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].expireAt.Before(entries[j].expireAt)
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	if c.stat != nil {
		c.stat.Add(context.Background(), MetricEvict, float64(evictItems), "name", c.config.Name)
	}

	for i := 0; i < evictItems; i++ {
		c.data.Delete(entries[i].key)
	}
}

func (c *xsyncMap) heapInUseOverflow() bool {
	if c.config.HeapInUseSoftLimit == 0 {
		return false
	}

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	return m.HeapInuse >= c.config.HeapInUseSoftLimit
}

func (c *xsyncMap) countOverflow() bool {
	if c.config.CountSoftLimit == 0 {
		return false
	}

	return c.Len() >= int(c.config.CountSoftLimit)
}
