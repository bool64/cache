package bench

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bool64/cache"
)

// Runner describes cache benchmark runner.
type Runner interface {
	Make(b *testing.B, cardinality int) (Runner, string)
	Run(b *testing.B, cnt int, writeEvery int)
}

// Default runners.
var (
	Failovers = []Runner{
		FailoverRunner{F: func() cache.ReadWriter {
			return cache.NewShardedMap()
		}},
		FailoverRunner{F: func() cache.ReadWriter {
			return cache.NewSyncMap()
		}},
	}

	Baseline = []Runner{
		SyncMapBaseline{},
		ShardedMapBaseline{},
		&MutexMap{},
		&RWMutexMap{},
	}

	ReadWriters = []Runner{
		ReadWriterRunner{Name: "ShardedMap/LRU", F: func() cache.ReadWriter {
			return cache.NewShardedMap(func(cfg *cache.Config) {
				cfg.CountSoftLimit = 1e10
				cfg.EvictionStrategy = cache.EvictLeastRecentlyUsed
			})
		}},
		ReadWriterRunner{F: func() cache.ReadWriter {
			return cache.NewShardedMap()
		}},
		ReadWriterRunner{F: func() cache.ReadWriter {
			return cache.NewSyncMap()
		}},
	}
)

// Scenario describes benchmark scenario.
type Scenario struct {
	Cardinality  int
	NumRoutines  int
	WritePercent float64
	Runners      []Runner
}

// Concurrently runs benchmarks concurrently.
func Concurrently(b *testing.B, scenarios []Scenario) {
	b.Helper()

	for _, sc := range scenarios {
		writeEvery := int(100.0 / sc.WritePercent)

		for _, loader := range sc.Runners {
			before := StableHeapInUse()
			now := time.Now()
			c, name := loader.Make(b, sc.Cardinality)
			preload := time.Since(now)
			inUse := float64(StableHeapInUse()-before) / (1024 * 1024)

			name = strings.ReplaceAll(name, "github.com/bool64/cache/bench.", "")

			b.Run(fmt.Sprintf("c%d:g%d:w%.2f%%:%s", sc.Cardinality, sc.NumRoutines, sc.WritePercent, name), func(b *testing.B) {
				c := c

				b.ReportAllocs()
				b.ResetTimer()

				wg := sync.WaitGroup{}
				wg.Add(sc.NumRoutines)

				for r := 0; r < sc.NumRoutines; r++ {
					cnt := b.N / sc.NumRoutines
					if r == 0 {
						cnt = b.N - cnt*(sc.NumRoutines-1)
					}

					go func() {
						defer wg.Done()
						c.Run(b, cnt, writeEvery)
					}()
				}

				wg.Wait()
				b.StopTimer()

				b.ReportMetric(inUse, "MB/inuse")                    // Memory footprint of preloaded data.
				b.ReportMetric(1000*preload.Seconds(), "ms/preload") // Time to populate initial data.
				fmt.Sprintln(c)
			})
		}
	}
}

// StableHeapInUse attempts to determine heap in use when new allocations are reduced.
func StableHeapInUse() int64 {
	var (
		m         = runtime.MemStats{}
		prevInUse uint64
		prevNumGC uint32
	)

	for {
		runtime.ReadMemStats(&m)

		// Considering heap stable if recent cycle collected less than 10KB.
		if prevNumGC != 0 && m.NumGC > prevNumGC && math.Abs(float64(m.HeapInuse-prevInUse)) < 10*1024 {
			break
		}

		prevInUse = m.HeapInuse
		prevNumGC = m.NumGC

		// Sleeping to allow GC to run a few times and collect all temporary data.
		time.Sleep(50 * time.Millisecond)

		runtime.GC()
	}

	return int64(m.HeapInuse)
}
