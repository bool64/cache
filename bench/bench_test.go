package bench_test

import (
	"runtime"
	"testing"

	"github.com/bool64/cache/bench"
)

func BenchmarkConcurrent(b *testing.B) {
	var all []bench.Runner

	all = append(all, bench.Baseline...)
	all = append(all, bench.ReadWriters...)
	all = append(all, bench.Failovers...)

	bench.Concurrently(b, []bench.Scenario{
		{Cardinality: 1e4, NumRoutines: 1, WritePercent: 0, Runners: bench.Failovers},                     // Fastest single-threaded mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0, Runners: bench.Failovers}, // Fastest mode.
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 0.1, Runners: bench.Failovers},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 1, Runners: bench.Failovers},
		{Cardinality: 1e4, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: bench.Failovers},
		{Cardinality: 1e6, NumRoutines: runtime.GOMAXPROCS(0), WritePercent: 10, Runners: all}, // Slowest mode.
	})
}
