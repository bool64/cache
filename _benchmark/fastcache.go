package benchmark

import (
	"strconv"
	"testing"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
)

// FastcacheBaseline is a benchmark runner.
type FastcacheBaseline struct {
	Encoding    cache.BinaryEncoding
	c           *fastcache.Cache
	cardinality int
}

func (r FastcacheBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c := fastcache.New(1e10)

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		v, err := r.Encoding.Encode(nil, bench.MakeCachedValue(i))
		if err != nil {
			b.Fatal("failed to encode in make", err)
		}

		c.Set(buf, v)
	}

	return FastcacheBaseline{
		Encoding:    r.Encoding,
		c:           c,
		cardinality: cardinality,
	}, "fastcache-base"
}

func (r FastcacheBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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
			r.c.Set(buf, v)
			r.c.Del(buf)

			continue
		}

		vb := r.c.Get(nil, buf)
		if vb == nil {
			b.Fatal("empty result in run")
		}

		v, err := r.Encoding.Decode(nil, vb)
		if err != nil {
			b.Fatal("failed to decode in run", err)
		}

		if v == nil || v.(bench.SmallCachedValue).I != i {
			b.Fatalf("val: %v", v)
		}

	}
}
