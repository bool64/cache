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

// ReadWriterOfRunner creates benchmark runner from cache.ReadWriterOf.
type ReadWriterOfRunner struct {
	f func() cache.ReadWriterOf[SmallCachedValue]

	be          cache.ReadWriterOf[SmallCachedValue]
	d           cache.Deleter
	cardinality int
}

// Make initializes benchmark runner.
func (cl ReadWriterOfRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := cl.f()
	ctx := context.Background()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		err := be.Write(ctx, buf, MakeCachedValue(i))
		if err != nil {
			b.Fail()
		}
	}

	return ReadWriterOfRunner{
		be:          be,
		d:           be.(cache.Deleter),
		cardinality: cardinality,
	}, fmt.Sprintf("%T", be)
}

// Run iterates over the cache.
func (cl ReadWriterOfRunner) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			err := cl.be.Write(ctx, buf, MakeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			if err = cl.d.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := cl.be.Read(ctx, buf)

		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
