package benchmark

import (
	"strconv"
	"testing"

	"github.com/bool64/cache/bench"
	"github.com/jeremiah-masters/dlht"
)

// DLHTBaseline is a benchmark runner.
type DLHTBaseline struct {
	c           *dlht.Map[string, bench.SmallCachedValue]
	cardinality int
}

func (r DLHTBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c := dlht.New[string, bench.SmallCachedValue](dlht.Options{})

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Insert(string(buf), bench.MakeCachedValue(i))
	}

	return DLHTBaseline{
		c:           c,
		cardinality: cardinality,
	}, "dlht.Map-base"
}

func (r DLHTBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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
			k := string(buf)

			r.c.Insert(k, bench.MakeCachedValue(i))
			r.c.Delete(k)

			continue
		}

		v, found := r.c.Get(string(buf))

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
