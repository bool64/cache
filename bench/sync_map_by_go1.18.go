//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/bool64/cache"
)

// SyncMapByBaseline is a benchmark runner.
type SyncMapByBaseline struct {
	c           *cache.SyncMapBy[string, SmallCachedValue]
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r SyncMapByBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewSyncMapBy[string, SmallCachedValue]()
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return SyncMapByBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "syncMapBy-base"
}

// Run iterates over the cache.
func (r SyncMapByBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality
		key := r.keys[i]

		w++
		if w == writeEvery {
			w = 0
			key = r.writeKeys[i]

			r.c.Store(key, MakeCachedValue(i))

			if err := r.c.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, found := r.c.Load(key)
		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
