package benchmark

import (
	"strconv"
	"testing"

	"github.com/bool64/cache/bench"
	"github.com/dgraph-io/ristretto"
)

// RistrettoBaseline is a benchmark runner.
type RistrettoBaseline struct {
	c           *ristretto.Cache
	cardinality int
}

func (r RistrettoBaseline) Make(b *testing.B, cardinality int) (bench.Runner, string) {
	b.Helper()

	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e6,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(bench.KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		if !c.Set(buf, bench.MakeCachedValue(i), 1) {
			c.Set(buf, bench.MakeCachedValue(i), 1)
		}
	}

	c.Wait()

	return RistrettoBaseline{
		c:           c,
		cardinality: cardinality,
	}, "ristretto-base"
}

func (r RistrettoBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

			r.c.Set(buf, bench.MakeCachedValue(i), 1)
			r.c.Del(buf)

			continue
		}

		v, found := r.c.Get(buf)

		if !found || v == nil || v.(bench.SmallCachedValue).I != i {
			// Ristretto rejects/evicts ~7-15% of the entries 🤷.

			continue
		}
	}
}
