//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"sync"
	"testing"
)

func Benchmark_ShardFunc_int_default(b *testing.B) {
	benchShardFuncInt(b, resolveShardFunc(ConfigBy[int]{}))
}

func Benchmark_ShardFunc_int_specialized(b *testing.B) {
	benchShardFuncInt(b, func(key int) uint64 {
		return mixInt64(int64(key))
	})
}

func Benchmark_ShardFunc_int_customConfig(b *testing.B) {
	benchShardFuncInt(b, resolveShardFunc(ConfigBy[int]{
		ShardFunc: func(key int) uint64 {
			return mixInt64(int64(key))
		},
	}))
}

func Benchmark_ShardedMapBy_int_default(b *testing.B) {
	benchShardedMapByIntConcurrent(b, nil)
}

func Benchmark_ShardedMapBy_int_specialized(b *testing.B) {
	benchShardedMapByIntConcurrent(b, func(key int) uint64 {
		return mixInt64(int64(key))
	})
}

func benchShardFuncInt(b *testing.B, shard func(int) uint64) {
	b.Helper()

	keys := make([]int, 1<<14)

	for i := range keys {
		keys[i] = i*17 + 3
	}

	var sink uint64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sink += shard(keys[i&(len(keys)-1)])
	}

	benchmarkSinkUint64 = sink
}

func benchShardedMapByIntConcurrent(b *testing.B, shard func(int) uint64) {
	b.Helper()

	c := NewShardedMapBy[int, int](func(cfg *ConfigBy[int]) {
		cfg.ShardFunc = shard
	})
	ctx := context.Background()

	cardinality := 10000
	keys := make([]int, cardinality)

	for i := 0; i < cardinality; i++ {
		k := i*17 + 3
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

var benchmarkSinkUint64 uint64
