package bench

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bool64/cache"
)

// FailoverRunner is a benchmark runner.
type FailoverRunner struct {
	F func() cache.ReadWriter

	C           *cache.Failover
	D           cache.Deleter
	Cardinality int
	Keys        [][]byte
	WriteKeys   [][]byte
}

// Make initializes benchmark runner.
func (r FailoverRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := r.F()
	ctx := context.Background()
	c := cache.NewFailover(cache.FailoverConfig{
		Backend: be,
	}.Use)
	keys, writeKeys := makeByteKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		i := i

		if _, err := c.Get(ctx, keys[i], func(ctx context.Context) (any, error) {
			return MakeCachedValue(i), nil
		}); err != nil {
			b.Fatal(err)
		}
	}

	return FailoverRunner{
		C:           c,
		D:           be.(cache.Deleter),
		Cardinality: cardinality,
		Keys:        keys,
		WriteKeys:   writeKeys,
	}, fmt.Sprintf("FailoverRunner(%T)", be)
}

// Run iterates over the cache.
func (r FailoverRunner) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.Cardinality
		key := r.Keys[i]

		w++
		if w == writeEvery {
			w = 0
			key = r.WriteKeys[i]

			v, err := r.C.Get(ctx, key, func(ctx context.Context) (any, error) {
				return MakeCachedValue(i), nil
			})

			if err != nil || v == nil || v.(SmallCachedValue).I != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = r.D.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v, key: %s", err, string(key))
			}

			continue
		}

		v, err := r.C.Get(ctx, key, func(ctx context.Context) (any, error) {
			panic("builder function should not be invoked while reading")
		})

		if err != nil || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
