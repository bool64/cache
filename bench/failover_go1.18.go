//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bool64/cache"
)

// FailoverOf is a benchmark runner.
type FailoverOf struct {
	F func() cache.ReadWriterOf[SmallCachedValue]

	C           *cache.FailoverOf[SmallCachedValue]
	D           cache.Deleter
	Cardinality int
	Keys        [][]byte
	WriteKeys   [][]byte
}

// Make initializes benchmark runner.
func (cl FailoverOf) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := cl.F()
	ctx := context.Background()
	c := cache.NewFailoverOf[SmallCachedValue](cache.FailoverConfigOf[SmallCachedValue]{
		Backend: be,
	}.Use)
	keys, writeKeys := makeByteKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		i := i

		if _, err := c.Get(ctx, keys[i], func(ctx context.Context) (SmallCachedValue, error) {
			return MakeCachedValue(i), nil
		}); err != nil {
			b.Fail()
		}
	}

	return FailoverOf{
		C:           c,
		D:           be.(cache.Deleter),
		Cardinality: cardinality,
		Keys:        keys,
		WriteKeys:   writeKeys,
	}, fmt.Sprintf("FailoverRunner(%T)", be)
}

// Run iterates over the cache.
func (cl FailoverOf) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.Cardinality
		key := cl.Keys[i]

		w++
		if w == writeEvery {
			w = 0
			key = cl.WriteKeys[i]

			v, err := cl.C.Get(ctx, key, func(ctx context.Context) (SmallCachedValue, error) {
				return MakeCachedValue(i), nil
			})

			if err != nil || v.I != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = cl.D.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v, key: %s", err, string(key))
			}

			continue
		}

		v, err := cl.C.Get(ctx, key, func(ctx context.Context) (SmallCachedValue, error) {
			panic("builder function should not be invoked while reading")
		})

		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
