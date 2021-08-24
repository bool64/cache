package cache_test

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bool64/cache"
)

func Benchmark_concurrent(b *testing.B) {
	dflt := []cacheLoader{
		failover{f: func() cache.ReadWriter {
			return cache.NewShardedMap()
		}},
	}

	// nolint:gocritic
	failovers := append(dflt,
		failover{f: func() cache.ReadWriter {
			return cache.NewSyncMap()
		}},
	)

	// nolint:gocritic
	all := append(failovers,
		backend{f: func() cache.ReadWriter {
			return cache.NewShardedMap()
		}},
		backend{f: func() cache.ReadWriter {
			return cache.NewSyncMap()
		}},

		syncMapBaseline{},
		shardedMapBaseline{},
	)

	type testcase struct {
		cardinality  int
		numRoutines  int
		writePercent float64
		cacheLoaders []cacheLoader
	}

	testcases := []testcase{
		{cardinality: 1e4, numRoutines: 1, writePercent: 0, cacheLoaders: dflt},                    // Fastest single-threaded mode.
		{cardinality: 1e4, numRoutines: runtime.GOMAXPROCS(0), writePercent: 0, cacheLoaders: all}, // Fastest mode.
		{cardinality: 1e4, numRoutines: runtime.GOMAXPROCS(0), writePercent: 0.1, cacheLoaders: dflt},
		{cardinality: 1e4, numRoutines: runtime.GOMAXPROCS(0), writePercent: 1, cacheLoaders: dflt},
		{cardinality: 1e4, numRoutines: runtime.GOMAXPROCS(0), writePercent: 10, cacheLoaders: dflt},
		{cardinality: 1e6, numRoutines: runtime.GOMAXPROCS(0), writePercent: 10, cacheLoaders: all}, // Slowest mode.
	}

	for _, tc := range testcases {
		writeEvery := int(100.0 / tc.writePercent)

		for _, loader := range tc.cacheLoaders {
			before := heapInUse()
			now := time.Now()
			c, name := loader.make(b, tc.cardinality)
			preload := time.Since(now)

			b.Run(fmt.Sprintf("c%d:g%d:w%.2f%%:%s", tc.cardinality, tc.numRoutines, tc.writePercent, name), func(b *testing.B) {
				c := c

				b.ReportAllocs()
				b.ResetTimer()

				wg := sync.WaitGroup{}
				wg.Add(tc.numRoutines)

				for r := 0; r < tc.numRoutines; r++ {
					cnt := b.N / tc.numRoutines
					if r == 0 {
						cnt = b.N - cnt*(tc.numRoutines-1)
					}

					go func() {
						defer wg.Done()
						c.run(b, cnt, writeEvery)
					}()
				}

				wg.Wait()
				b.StopTimer()

				b.ReportMetric(float64(heapInUse()-before)/(1024*1024), "MB/inuse")
				b.ReportMetric(1000*preload.Seconds(), "ms/preload")
				fmt.Sprintln(c)
			})
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
	make(b *testing.B, cardinality int) (cacheLoader, string)
	run(b *testing.B, cnt int, writeEvery int)
}

type backend struct {
	f func() cache.ReadWriter

	be          cache.ReadWriter
	cardinality int
}

func (cl backend) make(b *testing.B, cardinality int) (cacheLoader, string) {
	b.Helper()

	be := cl.f()
	ctx := context.Background()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		err := be.Write(ctx, buf, makeCachedValue(i))
		if err != nil {
			b.Fail()
		}
	}

	return backend{
		be:          be,
		cardinality: cardinality,
	}, fmt.Sprintf("%T", be)
}

func (cl backend) run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			err := cl.be.Write(ctx, buf, makeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := cl.be.Read(ctx, buf)

		if err != nil || v == nil || v.(smallCachedValue).i != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
