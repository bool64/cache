package bench

import (
	"context"
	"errors"
	"testing"

	"github.com/bool64/cache"
)

// ShardedMapBaseline is a benchmark runner.
type ShardedMapBaseline struct {
	c           *cache.ShardedMap
	cardinality int
	keys        [][]byte
	writeKeys   [][]byte
}

// Make initializes benchmark runner.
func (cl ShardedMapBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewShardedMap()
	keys, writeKeys := makeByteKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return ShardedMapBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "shardedMap-base"
}

// Run iterates over the cache.
func (cl ShardedMapBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

		if !found || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
