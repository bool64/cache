package bench

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

// FailoverRunner is a benchmark runner.
type FailoverRunner struct {
	F func() cache.ReadWriter

	C           *cache.Failover
	D           cache.Deleter
	Cardinality int
}

// Make initializes benchmark runner.
func (r FailoverRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := r.F()
	ctx := context.Background()
	c := cache.NewFailover(cache.FailoverConfig{
		Backend: be,
	}.Use)
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		_, err := c.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
			return MakeCachedValue(i), nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	return FailoverRunner{
		C:           c,
		D:           be.(cache.Deleter),
		Cardinality: cardinality,
	}, fmt.Sprintf("FailoverRunner(%T)", be)
}

// Run iterates over the cache.
func (r FailoverRunner) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.Cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			v, err := r.C.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
				return MakeCachedValue(i), nil
			})

			if err != nil || v == nil || v.(SmallCachedValue).I != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = r.D.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v, key: %s", err, string(buf))
			}

			continue
		}

		v, err := r.C.Get(ctx, buf, func(ctx context.Context) (interface{}, error) {
			panic("builder function should not be invoked while reading")
		})

		if err != nil || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
