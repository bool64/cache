package cache_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bool64/cache"
)

func ExampleNewShardedMap() {
	// Create cache instance.
	c := cache.NewShardedMap(cache.Config{
		Name:       "dogs",
		TimeToLive: 13 * time.Minute,
		// Logging errors with standard logger, non-error messages are ignored.
		Logger: cache.NewLogger(func(ctx context.Context, msg string, keysAndValues ...interface{}) {
			log.Printf("cache failed: %s %v", msg, keysAndValues)
		}, nil, nil, nil),

		// Tweak these parameters to reduce/stabilize rwMutexMap consumption at cost of cache hit rate.
		// If cache cardinality and size are reasonable, default values should be fine.
		DeleteExpiredAfter:       time.Hour,
		DeleteExpiredJobInterval: 10 * time.Minute,
		HeapInUseSoftLimit:       200 * 1024 * 1024, // 200MB soft limit for process heap in use.
		EvictFraction:            0.2,               // Drop 20% of mostly expired items (including non-expired) on heap overuse.
	}.Use)

	// Use context if available, it may hold TTL and SkipRead information.
	ctx := context.TODO()

	// Write value to cache.
	_ = c.Write(
		cache.WithTTL(ctx, time.Minute, true), // Change default TTL with context if necessary.
		[]byte("my-key"),
		[]int{1, 2, 3},
	)

	// Read value from cache.
	val, _ := c.Read(ctx, []byte("my-key"))
	fmt.Printf("%v", val)

	// Delete value from cache.
	_ = c.Delete(ctx, []byte("my-key"))

	// Output:
	// [1 2 3]
}

func ExampleFailover_Get() {
	ctx := context.TODO()
	f := cache.NewFailover()

	// Get value from cache or the function.
	v, err := f.Get(ctx, []byte("my-key"), func(ctx context.Context) (interface{}, error) {
		// Build value or return error on failure.

		return "<value>", nil
	})
	if err != nil {
		log.Fatal(err)
	}

	// Assert the type and use value.
	_ = v.(string)
}
