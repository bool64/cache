package bench

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

// ShardedMapBaseline is a benchmark runner.
type ShardedMapBaseline struct {
	c           *cache.ShardedMap
	cardinality int
}

// Make initializes benchmark runner.
func (cl ShardedMapBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewShardedMap()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Store(buf, MakeCachedValue(i))
	}

	return ShardedMapBaseline{
		c:           c,
		cardinality: cardinality,
	}, "shardedMap-base"
}

// Run iterates over the cache.
func (cl ShardedMapBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0
	ctx := context.Background()

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf := append(buf, 'n') // Insert new key.

			cl.c.Store(buf, MakeCachedValue(i))

			if err := cl.c.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, found := cl.c.Load(buf)

		if !found || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
