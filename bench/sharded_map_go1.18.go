//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

// ShardedMapOfBaseline is a benchmark runner.
type ShardedMapOfBaseline struct {
	c           *cache.ShardedMapOf[SmallCachedValue]
	cardinality int
}

// Make initializes benchmark runner.
func (cl ShardedMapOfBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := cache.NewShardedMapOf[SmallCachedValue]()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Store(buf, MakeCachedValue(i))
	}

	return ShardedMapOfBaseline{
		c:           c,
		cardinality: cardinality,
	}, "shardedMapOf-base"
}

// Run iterates over the cache.
func (cl ShardedMapOfBaseline) Run(b *testing.B, cnt int, writeEvery int) {
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

			buf = append(buf, 'n') // Insert new key.

			cl.c.Store(buf, MakeCachedValue(i))

			if err := cl.c.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, found := cl.c.Load(buf)

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
