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

// ReadWriterOfRunner creates benchmark runner from cache.ReadWriterOf.
type ReadWriterOfRunner struct {
	F func() cache.ReadWriterOf[SmallCachedValue]

	RW          cache.ReadWriterOf[SmallCachedValue]
	D           cache.Deleter
	Cardinality int
	Name        string
	Keys        [][]byte
	WriteKeys   [][]byte
}

// Make initializes benchmark runner.
func (cl ReadWriterOfRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := cl.F()
	ctx := context.Background()
	keys, writeKeys := makeByteKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		err := be.Write(ctx, keys[i], MakeCachedValue(i))
		if err != nil {
			b.Fail()
		}
	}

	name := cl.Name

	if name == "" {
		name = fmt.Sprintf("%T", be)
	}

	return ReadWriterOfRunner{
		RW:          be,
		D:           be.(cache.Deleter),
		Cardinality: cardinality,
		Keys:        keys,
		WriteKeys:   writeKeys,
	}, name
}

// Run iterates over the cache.
func (cl ReadWriterOfRunner) Run(b *testing.B, cnt int, writeEvery int) {
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

			err := cl.RW.Write(ctx, key, MakeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			if err = cl.D.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := cl.RW.Read(ctx, key)

		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
