package bench

import (
	"sync"
	"testing"
)

// SyncMapBaseline is a benchmark runner.
type SyncMapBaseline struct {
	c           *sync.Map
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r SyncMapBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := &sync.Map{}
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return SyncMapBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "sync.Map-base"
}

// Run iterates over the cache.
func (r SyncMapBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality
		key := r.keys[i]

		w++
		if w == writeEvery {
			w = 0

			key = r.writeKeys[i]

			r.c.Store(key, MakeCachedValue(i))
			r.c.Delete(key)

			continue
		}

		v, found := r.c.Load(key)

		if !found || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
