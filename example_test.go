package cache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
)

func ExampleNewShardedMap() {
	// Create cache instance.
	c := cache.NewShardedMap(cache.Config{
		Name:       "dogs",
		TimeToLive: 13 * time.Minute,
		Logger:     &ctxd.LoggerMock{},
		Stats:      &stats.TrackerMock{},

		// Tweak these parameters to reduce/stabilize rwMutexMap consumption at cost of cache hit rate.
		// If cache cardinality and size are reasonable, default values should be fine.
		DeleteExpiredAfter:       time.Hour,
		DeleteExpiredJobInterval: 10 * time.Minute,
		HeapInUseSoftLimit:       200 * 1024 * 1024, // 200MB soft limit for process heap in use.
		EvictFraction:            0.2,               // Drop 20% of mostly expired items (including non-expired) on heap overuse.
	}.Use)

	// Use context if available.
	ctx := context.TODO()

	// Write value to cache.
	_ = c.Write(ctx, []byte("my-key"), []int{1, 2, 3})

	// Read value from cache.
	val, _ := c.Read(ctx, []byte("my-key"))
	fmt.Printf("%v", val)

	// Output:
	// [1 2 3]
}
