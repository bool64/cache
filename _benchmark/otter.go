package benchmark

import (
	"github.com/bool64/cache/bench"
	"github.com/maypok86/otter"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)

// OtterBaseline is a benchmark runner.
type OtterBaseline struct {
	c           otter.Cache[string, bench.SmallCachedValue]
	cardinality int
}

func (r OtterBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()
	c, err := otter.MustBuilder[string, bench.SmallCachedValue](5 * cardinality).
		Build()
	require.NoError(b, err)

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Set(string(buf), bench.MakeCachedValue(i))
	}

	return OtterBaseline{
		c:           c,
		cardinality: cardinality,
	}, "otter.Map-base"
}

func (r OtterBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			r.c.Set(string(buf), bench.MakeCachedValue(i))
			r.c.Delete(string(buf))

			continue
		}

		v, found := r.c.Get(string(buf))

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
