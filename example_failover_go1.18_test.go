//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/bool64/cache"
)

// CachedGeneric is a service wrapper to serve already processed results from cache.
type CachedGeneric struct {
	upstream Service
	storage  *cache.FailoverOf[Value]
}

func (s CachedGeneric) GetByID(ctx context.Context, country string, id int) (Value, error) {
	// Prepare string cache key and call stale cache.
	// Build function will be called if there is a cache miss.
	cacheKey := country + ":" + strconv.Itoa(id)

	return s.storage.Get(ctx, []byte(cacheKey), func(ctx context.Context) (Value, error) {
		return s.upstream.GetByID(ctx, country, id)
	})
}

func ExampleFailoverOf_Get_caching_wrapper() {
	var service Service

	ctx := context.Background()

	service = CachedGeneric{
		upstream: &Real{},
		storage: cache.NewFailoverOf[Value](func(cfg *cache.FailoverConfigOf[Value]) {
			cfg.BackendConfig.TimeToLive = time.Minute
		}),
	}

	fmt.Println(service.GetByID(ctx, "DE", 123))
	fmt.Println(service.GetByID(ctx, "US", 0)) // Error increased sequence, but was cached with short-ttl.
	fmt.Println(service.GetByID(ctx, "US", 0)) // This call did not hit backend.
	fmt.Println(service.GetByID(ctx, "US", 456))
	fmt.Println(service.GetByID(ctx, "DE", 123))
	fmt.Println(service.GetByID(ctx, "US", 456))
	fmt.Println(service.GetByID(ctx, "FR", 789))

	// Output:
	// {1 123 DE} <nil>
	// {0 0 } invalid id
	// {0 0 } invalid id
	// {3 456 US} <nil>
	// {1 123 DE} <nil>
	// {3 456 US} <nil>
	// {4 789 FR} <nil>
}
