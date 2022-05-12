package bench

import (
	"strconv"
	"sync"
	"testing"
)

// SyncMapBaseline is a benchmark runner.
type SyncMapBaseline struct {
	c           *sync.Map
	cardinality int
}

// Make initializes benchmark runner.
func (r SyncMapBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := &sync.Map{}
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Store(string(buf), MakeCachedValue(i))
	}

	return SyncMapBaseline{
		c:           c,
		cardinality: cardinality,
	}, "sync.Map-base"
}

// Run iterates over the cache.
func (r SyncMapBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			r.c.Store(string(buf), MakeCachedValue(i))
			r.c.Delete(string(buf))

			continue
		}

		v, found := r.c.Load(string(buf))

		if !found || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
