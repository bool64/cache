package benchmark

import (
	"strconv"
	"testing"

	"github.com/bool64/cache/bench"
	"github.com/puzpuzpuz/xsync/v4"
)

// XsyncBaseline is a benchmark runner.
type XsyncBaseline struct {
	c           *xsync.Map[string, bench.SmallCachedValue]
	cardinality int
}

func (r XsyncBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c := xsync.NewMap[string, bench.SmallCachedValue]()

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Store(string(buf), bench.MakeCachedValue(i))
	}

	return XsyncBaseline{
		c:           c,
		cardinality: cardinality,
	}, "xsync.Map-base"
}

func (r XsyncBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

			r.c.Store(string(buf), bench.MakeCachedValue(i))
			r.c.Delete(string(buf))

			continue
		}

		v, found := r.c.Load(string(buf))

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
