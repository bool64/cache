package bench

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

// ReadWriterRunner is a benchmark runner.
type ReadWriterRunner struct {
	F func() cache.ReadWriter

	BE          cache.ReadWriter
	D           cache.Deleter
	Cardinality int
}

// Make initializes benchmark runner.
func (cl ReadWriterRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := cl.F()
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

	return ReadWriterRunner{
		BE:          be,
		D:           be.(cache.Deleter),
		Cardinality: cardinality,
	}, fmt.Sprintf("%T", be)
}

// Run iterates over the cache.
func (cl ReadWriterRunner) Run(b *testing.B, cnt int, writeEvery int) {
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

			err := cl.BE.Write(ctx, buf, MakeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			if err = cl.D.Delete(ctx, buf); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := cl.BE.Read(ctx, buf)

		if err != nil || v == nil || v.(SmallCachedValue).I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
