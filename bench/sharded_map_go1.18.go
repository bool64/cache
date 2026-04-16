//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/bool64/cache"
)

// ShardedMapOfBaseline is a benchmark runner.
type ShardedMapOfBaseline struct {
	c           *cache.ShardedMapOf[SmallCachedValue]
	cardinality int
	keys        [][]byte
	writeKeys   [][]byte
}

// Make initializes benchmark runner.
func (cl ShardedMapOfBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewShardedMapOf[SmallCachedValue]()
	keys, writeKeys := makeByteKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return ShardedMapOfBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "shardedMapOf-base"
}

// Run iterates over the cache.
func (cl ShardedMapOfBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	w := 0
	ctx := context.Background()

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality
		key := cl.keys[i]

		w++
		if w == writeEvery {
			w = 0

			key = cl.writeKeys[i]

			cl.c.Store(key, MakeCachedValue(i))

			if err := cl.c.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, found := cl.c.Load(key)

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
