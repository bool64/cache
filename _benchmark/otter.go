package benchmark

import (
	"context"
	"strconv"
	"testing"

	"github.com/bool64/cache/bench"
	"github.com/maypok86/otter/v2"
)

// OtterBaseline is a benchmark runner.
type OtterBaseline struct {
	c           *otter.Cache[string, bench.SmallCachedValue]
	cardinality int
}

func (r OtterBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c := otter.Must[string, bench.SmallCachedValue](&otter.Options[string, bench.SmallCachedValue]{
		MaximumSize: cardinality,
	})

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
	}, "otter.Cache-base"
}

func (r OtterBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0
	ctx := context.Background()

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.
			k := string(buf)

			r.c.Set(k, bench.MakeCachedValue(i))
			r.c.Invalidate(k)

			continue
		}

		v, err := r.c.Get(ctx, string(buf), otter.LoaderFunc[string, bench.SmallCachedValue](func(ctx context.Context, key string) (bench.SmallCachedValue, error) {
			return bench.MakeCachedValue(i), nil
		}))

		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
