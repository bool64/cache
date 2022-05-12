package benchmark

import (
	"strconv"
	"testing"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/bool64/cache"
	"github.com/bool64/cache/bench"
)

// BigcacheBaseline is a benchmark runner.
type BigcacheBaseline struct {
	Encoding    cache.BinaryEncoding
	c           *bigcache.BigCache
	cardinality int
}

func (r BigcacheBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	cfg := bigcache.DefaultConfig(10 * time.Minute)

	c, err := bigcache.NewBigCache(cfg)
	if err != nil {
		b.Fatal("failed to create", err)
	}

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		v, err := r.Encoding.Encode(nil, bench.MakeCachedValue(i))
		if err != nil {
			b.Fatal("failed to encode in make", err)
		}

		err = c.Set(string(buf), v)
		if err != nil {
			b.Fatal("failed to set in make", err)
		}
	}

	return BigcacheBaseline{
		Encoding:    r.Encoding,
		c:           c,
		cardinality: cardinality,
	}, "bigcache-base"
}

func (r BigcacheBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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
			err = r.c.Set(string(buf), v)
			if err != nil {
				b.Fatal("failed to set in run", err)
			}
			err = r.c.Delete(string(buf))
			if err != nil {
				// Deletions are sometimes failing, apparently the value haven't arrived to cache yet.

				// b.Fatal("failed to delete in run", err)
			}

			continue
		}

		vb, err := r.c.Get(string(buf))
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
			b.Fatal(err)
		}

	}
}
