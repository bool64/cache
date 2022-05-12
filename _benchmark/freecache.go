package benchmark

import (
	"strconv"
	"testing"

	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
	"github.com/coocood/freecache"
)

// FreecacheBaseline is a benchmark runner.
type FreecacheBaseline struct {
	Encoding    cache.BinaryEncoding
	c           *freecache.Cache
	cardinality int
}

func (r FreecacheBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	cacheSize := 300 * 1024 * 1024
	c := freecache.NewCache(cacheSize)

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		v, err := r.Encoding.Encode(nil, bench.MakeCachedValue(i))
		if err != nil {
			b.Fatal("failed to encode in make", err)
		}

		err = c.Set(buf, v, 1000)
		if err != nil {
			b.Fatal("failed to set in make", err)
		}
	}

	return FreecacheBaseline{
		Encoding:    r.Encoding,
		c:           c,
		cardinality: cardinality,
	}, "freecache-base"
}

func (r FreecacheBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

			buf := append(buf, 'n') // Insert new key.

			v, err := r.Encoding.Encode(nil, bench.MakeCachedValue(i))
			if err != nil {
				b.Fatal("failed to encode in run", err)
			}
			err = r.c.Set(buf, v, 1000)
			if err != nil {
				b.Fatal("failed to set in run", err)
			}
			r.c.Del(buf)

			continue
		}

		vb, err := r.c.Get(buf)
		if err == nil {
			v, err := r.Encoding.Decode(nil, vb)
			if err != nil {
				b.Fatal("failed to decode in run", err)
			}

			if v == nil || v.(bench.SmallCachedValue).I != i {
				b.Fatalf("val: %v", v)
			}
		}

		if err != nil {
			b.Fatal("failed to get in run", err)
		}
	}
}
