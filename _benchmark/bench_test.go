package benchmark_test

import (
	"benchmark"
	"encoding"
	"runtime"
	"testing"

	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
)

func BenchmarkConcurrentBaseline(b *testing.B) {
	var base []bench.Runner

	base = append(base, bench.Baseline...)

	enc := cache.BinaryUnmarshaler(func(data []byte) (encoding.BinaryMarshaler, error) {
		v := bench.SmallCachedValue{}
		err := v.UnmarshalBinary(data)

		return v, err
	})

	base = append(base,
		benchmark.RistrettoBaseline{},
		benchmark.XsyncBaseline{},
		benchmark.PatrickmnBaseline{},
		benchmark.BigcacheBaseline{Encoding: enc},
		benchmark.FreecacheBaseline{Encoding: enc},
		benchmark.FastcacheBaseline{Encoding: enc},
	)

	bench.Concurrently(b, []bench.Scenario{
		{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: base},                     // Fastest single-threaded mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: base}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: base},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: base},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: base},
		{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: base}, // Slowest mode.
	})
}

func BenchmarkConcurrentFailover(b *testing.B) {
	var base []bench.Runner

	base = append(base, bench.Failovers...)

	bench.Concurrently(b, []bench.Scenario{
		{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: base},                     // Fastest single-threaded mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: base}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: base},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: base},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: base},
		{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: base}, // Slowest mode.
	})
}
