package cache_test

import (
	"context"
	"encoding/binary"
	"math"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bool64/cache"
)

func Benchmark_ShardedByteMap_concurrent(b *testing.B) {
	c := cache.NewShardedMap()
	ctx := context.Background()

	cardinality := 10000
	for i := 0; i < cardinality; i++ {
		k := "oneone" + strconv.Itoa(i)
		_ = c.Write(ctx, []byte(k), 123)
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
				k := "oneone" + strconv.Itoa((i^12345)%cardinality)
				v, _ := c.Read(ctx, []byte(k))

				if v.(int) != 123 {
					b.Fail()
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func heapInUse() int64 {
	var (
		m         = runtime.MemStats{}
		prevInUse uint64
		prevNumGC uint32
	)

	for {
		runtime.ReadMemStats(&m)

		if prevNumGC != 0 && math.Abs(float64(m.HeapInuse-prevInUse)) < 10*1024 {
			break
		}

		prevInUse = m.HeapInuse
		prevNumGC = m.NumGC

		time.Sleep(50 * time.Millisecond)

		runtime.GC()
	}

	return int64(m.HeapInuse)
}

// Benchmark_Failover_noSyncRead-8   	 7716646	       148.8 ns/op	       0 B/op	       0 allocs/op.
func Benchmark_Failover_noSyncRead(b *testing.B) {
	c := cache.NewFailover()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	k := make([]byte, 0, 10)

	for i := 0; i < b.N; i++ {
		k = append(k[:0], []byte("oneone1234")...)
		binary.BigEndian.PutUint32(k[6:], uint32(i%10000))
		_, _ = c.Get(ctx, k, func(ctx context.Context) (interface{}, error) {
			return 123, nil
		})
	}
}

// Benchmark_FailoverSyncRead-8   	 3764518	       321.5 ns/op	     113 B/op	       2 allocs/op.
func Benchmark_FailoverSyncRead(b *testing.B) {
	c := cache.NewFailover(func(cfg *cache.FailoverConfig) {
		cfg.SyncRead = true
	})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	k := make([]byte, 0, 10)

	for i := 0; i < b.N; i++ {
		k = append(k[:0], []byte("oneone1234")...)
		binary.BigEndian.PutUint32(k[6:], uint32(i%10000))

		_, _ = c.Get(ctx, k, func(ctx context.Context) (interface{}, error) {
			return 123, nil
		})
	}
}

// Benchmark_FailoverAlwaysBuild-8   	 1000000	      1379 ns/op	     399 B/op	      10 allocs/op.
func Benchmark_FailoverAlwaysBuild(b *testing.B) {
	c := cache.NewFailover(func(cfg *cache.FailoverConfig) {
		cfg.SyncUpdate = true
	})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	k := make([]byte, 0, 10)

	for i := 0; i < b.N; i++ {
		k = append(k[:0], []byte("oneone1234")...)
		binary.BigEndian.PutUint32(k[6:], uint32(i%10000))

		_, _ = c.Get(cache.WithTTL(ctx, cache.SkipWriteTTL, false), k, func(ctx context.Context) (interface{}, error) {
			return 123, nil
		})
	}
}
