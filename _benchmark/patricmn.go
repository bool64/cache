package benchmark

import (
	"strconv"
	"testing"
	"time"

	"github.com/bool64/cache/bench"
	cache "github.com/patrickmn/go-cache"
)

// PatrickmnBaseline is a benchmark runner.
type PatrickmnBaseline struct {
	c           *cache.Cache
	cardinality int
}

func (r PatrickmnBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c := cache.New(5*time.Minute, 10*time.Minute)

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Set(string(buf), bench.MakeCachedValue(i), time.Minute)
	}

	return PatrickmnBaseline{
		c:           c,
		cardinality: cardinality,
	}, "patrickmn-base"
}

func (r PatrickmnBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

			r.c.Set(string(buf), bench.MakeCachedValue(i), time.Minute)
			r.c.Delete(string(buf))

			continue
		}

		v, found := r.c.Get(string(buf))

		if !found || v == nil || v.(bench.SmallCachedValue).I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
