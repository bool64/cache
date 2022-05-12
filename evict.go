package cache

import (
	"context"
	"runtime"
	"sort"
	"time"
)

type evictEntry struct {
	hash     uint64
	expireAt time.Time
}

func (c *shardedMap) evictOldest() {
	evictFraction := c.t.Config.EvictFraction
	if evictFraction == 0 {
		evictFraction = 0.1
	}

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

	if c.t.stat != nil {
		c.t.stat.Add(context.Background(), MetricEvict, float64(evictItems), "name", c.t.Config.Name)
	}

	for i := 0; i < evictItems; i++ {
		h := entries[i].hash
		b := &c.hashedBuckets[h%shards]

		b.Lock()
		delete(b.data, h)
		b.Unlock()
	}
}

func heapInUseOverflow(c Config) bool {
	if c.HeapInUseSoftLimit == 0 {
		return false
	}

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	return m.HeapInuse >= c.HeapInUseSoftLimit
}

func countOverflow(c Config, length func() int) bool {
	if c.CountSoftLimit == 0 {
		return false
	}

	return length() >= int(c.CountSoftLimit)
}
