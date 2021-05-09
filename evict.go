package cache

import (
	"context"
	"runtime"
	"sort"
	"time"
)

func (c *shardedMap) evictOldest() {
	evictFraction := c.config.EvictFraction
	if evictFraction == 0 {
		evictFraction = 0.1
	}

	type entry struct {
		key      string
		hash     uint64
		expireAt time.Time
	}

	keysCnt := c.Len()
	entries := make([]entry, 0, keysCnt)

	// Collect all keys and expirations.
	for i := range c.hashedBuckets {
		b := &c.hashedBuckets[i]

		b.RLock()
		for h, i := range b.data {
			entries = append(entries, entry{hash: h, expireAt: i.E, key: string(i.K)})
		}
		b.RUnlock()
	}

	// Sort entries to put most expired in head.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].expireAt.Before(entries[j].expireAt)
	})

	evictItems := int(float64(len(entries)) * evictFraction)

	if c.stat != nil {
		c.stat.Add(context.Background(), MetricEvict, float64(evictItems), "name", c.config.Name)
	}

	for i := 0; i < evictItems; i++ {
		h := entries[i].hash
		b := &c.hashedBuckets[h%shards]

		b.Lock()
		delete(b.data, h)
		b.Unlock()
	}
}

func (c *shardedMap) heapInUseOverflow() bool {
	if c.config.HeapInUseSoftLimit == 0 {
		return false
	}

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	return m.HeapInuse >= c.config.HeapInUseSoftLimit
}

func (c *shardedMap) countOverflow() bool {
	if c.config.CountSoftLimit == 0 {
		return false
	}

	return c.Len() >= int(c.config.CountSoftLimit)
}
