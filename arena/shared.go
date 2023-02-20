package arena

import (
	"context"
	"github.com/bool64/cache"
	"time"
)

type evictLeastEntry struct {
	hash uint64
	val  int64
}

func expireAt(ctx context.Context, c cache.Trait) (time.Duration, int64) {
	if ttl := c.TTL(ctx); ttl != 0 {
		return ttl, ts(time.Now().Add(ttl))
	}

	return 0, 0
}

func ts(t time.Time) int64 {
	return t.UnixNano()
}

func tsTime(ns int64) time.Time {
	return time.Unix(ns/1e9, ns%1e9)
}
