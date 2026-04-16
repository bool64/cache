//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/bool64/cache"
)

func Benchmark_ShardedMapBy_concurrent(b *testing.B) {
	c := cache.NewShardedMapBy[string, int]()
	ctx := context.Background()

	cardinality := 10000
	keys := make([]string, cardinality)

	for i := 0; i < cardinality; i++ {
		k := "oneone" + strconv.Itoa(i)
		keys[i] = k
		_ = c.Write(ctx, k, 123)
	}

	b.ReportAllocs()
	b.ResetTimer()

	numRoutines := 50
	wg := sync.WaitGroup{}
	wg.Add(numRoutines)

	for r := 0; r < numRoutines; r++ {
		cnt := b.N / numRoutines
		if r == 0 {
			cnt = b.N - cnt*(numRoutines-1)
		}

		go func() {
			for i := 0; i < cnt; i++ {
				k := keys[(i^12345)%cardinality]
				v, _ := c.Read(ctx, k)

				if v != 123 {
					b.Fail()
				}
			}

			wg.Done()
		}()
	}

	wg.Wait()
}

func Benchmark_ShardedMapOf_concurrent(b *testing.B) {
	c := cache.NewShardedMapOf[int]()
	ctx := context.Background()

	cardinality := 10000
	keys := make([][]byte, cardinality)

	for i := 0; i < cardinality; i++ {
		k := "oneone" + strconv.Itoa(i)
		keys[i] = []byte(k)
		_ = c.Write(ctx, keys[i], 123)
	}

	b.ReportAllocs()
	b.ResetTimer()

	numRoutines := 50
	wg := sync.WaitGroup{}
	wg.Add(numRoutines)

	for r := 0; r < numRoutines; r++ {
		cnt := b.N / numRoutines
		if r == 0 {
			cnt = b.N - cnt*(numRoutines-1)
		}

		go func() {
			for i := 0; i < cnt; i++ {
				k := keys[(i^12345)%cardinality]
				v, _ := c.Read(ctx, k)

				if v != 123 {
					b.Fail()
				}
			}

			wg.Done()
		}()
	}

	wg.Wait()
}

func Benchmark_FailoverBy_noSyncRead(b *testing.B) {
	c := cache.NewFailoverBy[string, int]()
	ctx := context.Background()
	keys := make([]string, 10000)

	for i := range keys {
		keys[i] = "oneone" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		_, _ = c.Get(ctx, k, func(ctx context.Context) (int, error) {
			return 123, nil
		})
	}
}

func Benchmark_FailoverOf_noSyncRead(b *testing.B) {
	c := cache.NewFailoverOf[int]()
	ctx := context.Background()
	keys := make([][]byte, 10000)

	for i := range keys {
		keys[i] = []byte("oneone" + strconv.Itoa(i))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		_, _ = c.Get(ctx, k, func(ctx context.Context) (int, error) {
			return 123, nil
		})
	}
}

func Benchmark_FailoverBySyncRead(b *testing.B) {
	c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
		cfg.SyncRead = true
	})
	ctx := context.Background()
	keys := make([]string, 10000)

	for i := range keys {
		keys[i] = "oneone" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		_, _ = c.Get(ctx, k, func(ctx context.Context) (int, error) {
			return 123, nil
		})
	}
}

func Benchmark_FailoverOfSyncRead(b *testing.B) {
	c := cache.NewFailoverOf[int](func(cfg *cache.FailoverConfigOf[int]) {
		cfg.SyncRead = true
	})
	ctx := context.Background()
	keys := make([][]byte, 10000)

	for i := range keys {
		keys[i] = []byte("oneone" + strconv.Itoa(i))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		_, _ = c.Get(ctx, k, func(ctx context.Context) (int, error) {
			return 123, nil
		})
	}
}
