package benchmark

import (
	"context"
	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
	"golang.org/x/sync/singleflight"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

var sfSleep = 10 * time.Microsecond

func BenchmarkSingleflight(b *testing.B) {
	sf := singleflight.Group{}
	cardinality := 100
	numRoutines := runtime.GOMAXPROCS(0)

	wg := sync.WaitGroup{}
	wg.Add(numRoutines)

	totalCalls := int64(0)

	b.ReportAllocs()

	for r := 0; r < numRoutines; r++ {
		cnt := b.N / numRoutines
		if r == 0 {
			cnt = b.N - cnt*(numRoutines-1)
		}

		go func() {
			defer wg.Done()

			buf := make([]byte, 0, 10)

			for i := 0; i < cnt; i++ {
				i := (i ^ 12345) % cardinality

				buf = append(buf[:0], []byte(bench.KeyPrefix)...)
				buf = append(buf, []byte(strconv.Itoa(i))...)

				_, _, _ = sf.Do(bytesToString(buf), func() (interface{}, error) {
					time.Sleep(sfSleep)
					atomic.AddInt64(&totalCalls, 1)
					return nil, nil
				})
			}
		}()
	}

	wg.Wait()
}

func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func BenchmarkSfGet(b *testing.B) {
	cardinality := 100
	numRoutines := runtime.GOMAXPROCS(0)

	c := cache.NewFailover(func(cfg *cache.FailoverConfig) {
		cfg.Backend = cache.NoOp{}
	})

	wg := sync.WaitGroup{}
	wg.Add(numRoutines)

	totalCalls := int64(0)

	b.ReportAllocs()
	ctx := context.Background()

	for r := 0; r < numRoutines; r++ {
		cnt := b.N / numRoutines
		if r == 0 {
			cnt = b.N - cnt*(numRoutines-1)
		}

		go func() {
			defer wg.Done()

			buf := make([]byte, 0, 10)

			for i := 0; i < cnt; i++ {
				i := (i ^ 12345) % cardinality

				buf = append(buf[:0], []byte(bench.KeyPrefix)...)
				buf = append(buf, []byte(strconv.Itoa(i))...)

				_, _ = c.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
					time.Sleep(sfSleep)
					atomic.AddInt64(&totalCalls, 1)
					return nil, nil
				})
			}
		}()
	}

	wg.Wait()
}

func BenchmarkSfGetOf(b *testing.B) {
	cardinality := 100
	numRoutines := runtime.GOMAXPROCS(0)

	c := cache.NewFailoverOf[int](func(cfg *cache.FailoverConfigOf[int]) {
		cfg.Backend = cache.NoOpOf[int]{}
	})

	wg := sync.WaitGroup{}
	wg.Add(numRoutines)

	totalCalls := int64(0)

	b.ReportAllocs()
	ctx := context.Background()

	for r := 0; r < numRoutines; r++ {
		cnt := b.N / numRoutines
		if r == 0 {
			cnt = b.N - cnt*(numRoutines-1)
		}

		go func() {
			defer wg.Done()

			buf := make([]byte, 0, 10)

			for i := 0; i < cnt; i++ {
				i := (i ^ 12345) % cardinality

				buf = append(buf[:0], []byte(bench.KeyPrefix)...)
				buf = append(buf, []byte(strconv.Itoa(i))...)

				_, _ = c.Get(ctx, buf, func(ctx context.Context) (int, error) {
					time.Sleep(sfSleep)
					atomic.AddInt64(&totalCalls, 1)
					return 0, nil
				})
			}
		}()
	}

	wg.Wait()
}
