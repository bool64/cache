//go:build go1.18
// +build go1.18

package bench

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

// FailoverOf is a benchmark runner.
type FailoverOf struct {
	F func() cache.ReadWriterOf[SmallCachedValue]

	C           *cache.FailoverOf[SmallCachedValue]
	d           cache.Deleter
	Cardinality int
}

// Make initializes benchmark runner.
func (cl FailoverOf) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := cl.F()
	ctx := context.Background()
	c := cache.NewFailoverOf[SmallCachedValue](cache.FailoverConfigOf[SmallCachedValue]{
		Backend: be,
	}.Use)
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		_, err := c.Get(ctx, buf, func(ctx context.Context) (SmallCachedValue, error) {
			return MakeCachedValue(i), nil
		})
		if err != nil {
			b.Fail()
		}
	}

	return FailoverOf{
		C:           c,
		d:           be.(cache.Deleter),
		Cardinality: cardinality,
	}, fmt.Sprintf("FailoverRunner(%T)", be)
}

// Run iterates over the cache.
func (cl FailoverOf) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.Cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			v, err := cl.C.Get(ctx, buf, func(ctx context.Context) (SmallCachedValue, error) {
				return MakeCachedValue(i), nil
			})

			if err != nil || v.I != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = cl.d.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v, key: %s", err, string(buf))
			}

			continue
		}

		v, err := cl.C.Get(ctx, buf, func(ctx context.Context) (SmallCachedValue, error) {
			panic("builder function should not be invoked while reading")
		})

		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
