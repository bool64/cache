package benchmark_test

import (
	"encoding"
	"runtime"
	"testing"

	"benchmark"

	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
)

func BenchmarkConcurrentBaseline(b *testing.B) {
	var runners []bench.Runner

	runners = append(runners, bench.Baseline...)

	enc := cache.BinaryUnmarshaler(func(data []byte) (encoding.BinaryMarshaler, error) {
		v := bench.SmallCachedValue{}
		err := v.UnmarshalBinary(data)

		return v, err
	})

	runners = append(runners,
		benchmark.RistrettoBaseline{},
		benchmark.XsyncBaseline{},
		benchmark.OtterBaseline{},
		//benchmark.PatrickmnBaseline{},
		//benchmark.BigcacheBaseline{Encoding: enc},
		//benchmark.FreecacheBaseline{Encoding: enc},
		benchmark.FastcacheBaseline{Encoding: enc},
	)

	bench.Concurrently(b, []bench.Scenario{
		//{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: runners},                     // Fastest single-threaded mode.
		//{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: runners}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: runners},
		//{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: runners},
		//{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners},
		//{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners}, // Slowest mode.
	})
}

func BenchmarkConcurrentFailover(b *testing.B) {
	var runners []bench.Runner

	runners = append(runners, bench.Failovers...)

	bench.Concurrently(b, []bench.Scenario{
		{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: runners},                     // Fastest single-threaded mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: runners}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: runners},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: runners},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners},
		{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners}, // Slowest mode.
	})
}

func BenchmarkConcurrentReadWriter(b *testing.B) {
	var runners []bench.Runner

	runners = append(runners, bench.ReadWriters...)

	bench.Concurrently(b, []bench.Scenario{
		{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: runners},                     // Fastest single-threaded mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: runners}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: runners},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: runners},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners},
		{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: runners}, // Slowest mode.
	})
}
