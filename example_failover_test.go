package cache_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bool64/cache"
)

type Value struct {
	Sequence int
	ID       int
	Country  string
}

type Service interface {
	GetByID(ctx context.Context, country string, id int) (Value, error)
}

// Real is an instance of real service that does some slow/expensive processing.
type Real struct {
	Calls int
}

func (s *Real) GetByID(ctx context.Context, country string, id int) (Value, error) {
	s.Calls++

	if id == 0 {
		return Value{}, errors.New("invalid id")
	}

	return Value{
		Country:  country,
		ID:       id,
		Sequence: s.Calls,
	}, nil
}

// Cached is a service wrapper to serve already processed results from cache.
type Cached struct {
	upstream Service
	storage  *cache.Failover
}

func (s Cached) GetByID(ctx context.Context, country string, id int) (Value, error) {
	// Prepare string cache key and call stale cache.
	// Build function will be called if there is a cache miss.
	cacheKey := country + ":" + strconv.Itoa(id)

	value, err := s.storage.Get(ctx, []byte(cacheKey), func(ctx context.Context) (interface{}, error) {
		return s.upstream.GetByID(ctx, country, id)
	})
	if err != nil {
		return Value{}, err
	}

	// Type assert and return result.
	return value.(Value), nil
}

func ExampleFailover_Get() {
	var service Service

	ctx := context.Background()

	service = Cached{
		upstream: &Real{},
		storage: cache.NewFailover(func(cfg *cache.FailoverConfig) {
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
