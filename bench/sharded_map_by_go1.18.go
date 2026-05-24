//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/bool64/cache"
)

// ShardedMapByBaseline is a benchmark runner.
type ShardedMapByBaseline struct {
	c           *cache.ShardedMapBy[string, SmallCachedValue]
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r ShardedMapByBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewShardedMapBy[string, SmallCachedValue]()
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return ShardedMapByBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "shardedMapBy-base"
}

// Run iterates over the cache.
func (r ShardedMapByBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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
