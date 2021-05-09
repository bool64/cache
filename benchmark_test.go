package cache_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bool64/cache"
)

func Benchmark_concurrentRead(b *testing.B) {
	for _, cardinality := range []int{1e4} {
		cardinality := cardinality

		for _, numRoutines := range []int{1, runtime.GOMAXPROCS(0)} {
			numRoutines := numRoutines

			for _, loader := range []cacheLoader{
				failoverShardedMap{},
			} {
				loader := loader

				b.Run(fmt.Sprintf("%d:%d:%T", cardinality, numRoutines, loader), func(b *testing.B) {
					before := heapInUse()

					c := loader.make(b, cardinality)

					b.ReportAllocs()
					b.ResetTimer()

					wg := sync.WaitGroup{}
					wg.Add(numRoutines)

					for r := 0; r < numRoutines; r++ {
						cnt := b.N / numRoutines
						if r == 0 {
							cnt = b.N - cnt*(numRoutines-1)
						}

						go func() {
							c.run(b, cnt)
							wg.Done()
						}()
					}

					wg.Wait()
					b.StopTimer()
					b.ReportMetric(float64(heapInUse()-before)/(1024*1024), "MB/inuse")
					fmt.Sprintln(c)
				})
			}
		}
	}
}

// cachedValue represents a small value for a cached item.
type smallCachedValue struct {
	b bool
	s string
	i int
}

func makeCachedValue(i int) smallCachedValue {
	return smallCachedValue{
		i: i,
		s: longString + strconv.Itoa(i),
		b: true,
	}
}

func init() {
	cache.GobRegister(smallCachedValue{})
}

const (
	longString = "looooooooooooooooooooooooooongstring"
	keyPrefix  = "thekey"
)

type cacheLoader interface {
	make(b *testing.B, cardinality int) cacheLoader
	run(b *testing.B, cnt int)
}

type failoverShardedMap struct {
	c           *cache.Failover
	cardinality int
}

func (sbm failoverShardedMap) make(b *testing.B, cardinality int) cacheLoader {
	b.Helper()

	u := cache.NewShardedMap()
	ctx := context.Background()
	c := cache.NewFailover(func(cfg *cache.FailoverConfig) {
		cfg.Backend = u
	})
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		_, err := c.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
			return makeCachedValue(i), nil
		})
		if err != nil {
			b.Fail()
		}
	}

	return failoverShardedMap{
		c:           c,
		cardinality: cardinality,
	}
}

func (sbm failoverShardedMap) run(b *testing.B, cnt int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % sbm.cardinality

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		v, err := sbm.c.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
			return smallCachedValue{}, nil
		})

		if v.(smallCachedValue).i != i || err != nil {
			b.Fail()
		}
	}
}

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

func heapInUse() uint64 {
	var (
		m         = runtime.MemStats{}
		prevInUse uint64
	)

	for {
		runtime.ReadMemStats(&m)

		if math.Abs(float64(m.HeapInuse-prevInUse)) < 1*1024 {
			break
		}

		prevInUse = m.HeapInuse

		time.Sleep(50 * time.Millisecond)
		runtime.GC()
		debug.FreeOSMemory()
	}

	return m.HeapInuse
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
