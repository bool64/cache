//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"sort"
)

func (c *shardedMapOf[V]) evictOldest() {
	evictFraction := c.t.Config.EvictFraction
	if evictFraction == 0 {
		evictFraction = 0.1
	}

	keysCnt := c.Len()
	entries := make([]evictEntry, 0, keysCnt)

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

	if c.t.Stat != nil {
		c.t.Stat.Add(context.Background(), MetricEvict, float64(evictItems), "name", c.t.Config.Name)
	}

	for i := 0; i < evictItems; i++ {
		h := entries[i].hash
		b := &c.hashedBuckets[h%shards]

		b.Lock()
		delete(b.data, h)
		b.Unlock()
	}
}
