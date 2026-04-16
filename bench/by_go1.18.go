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

// ReadWriterByRunner is a benchmark runner.
type ReadWriterByRunner struct {
	F func() cache.ReadWriterBy[string, SmallCachedValue]

	RW cache.ReadWriterBy[string, SmallCachedValue]
	D  interface {
		Delete(ctx context.Context, key string) error
	}
	Cardinality int
	Name        string
	Keys        []string
	WriteKeys   []string
}

// Make initializes benchmark runner.
func (r ReadWriterByRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := r.F()
	ctx := context.Background()
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		err := be.Write(ctx, keys[i], MakeCachedValue(i))
		if err != nil {
			b.Fatal(err)
		}
	}

	name := r.Name
	if name == "" {
		name = fmt.Sprintf("%T", be)
	}

	return ReadWriterByRunner{
		RW: be,
		D: be.(interface {
			Delete(ctx context.Context, key string) error
		}),
		Cardinality: cardinality,
		Keys:        keys,
		WriteKeys:   writeKeys,
	}, name
}

// Run iterates over the cache.
func (r ReadWriterByRunner) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.Cardinality
		k := r.Keys[i]

		w++
		if w == writeEvery {
			w = 0

			k = r.WriteKeys[i]

			err := r.RW.Write(ctx, k, MakeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			if err = r.D.Delete(ctx, k); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := r.RW.Read(ctx, k)
		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}

// FailoverByRunner is a benchmark runner.
type FailoverByRunner struct {
	F func() cache.ReadWriterBy[string, SmallCachedValue]

	C *cache.FailoverBy[string, SmallCachedValue]
	D interface {
		Delete(ctx context.Context, key string) error
	}
	Cardinality int
	Keys        []string
	WriteKeys   []string
}

// Make initializes benchmark runner.
func (r FailoverByRunner) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	be := r.F()
	ctx := context.Background()
	c := cache.NewFailoverBy[string, SmallCachedValue](cache.FailoverConfigBy[string, SmallCachedValue]{
		Backend: be,
	}.Use)
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		i := i
		k := keys[i]

		_, err := c.Get(ctx, k, func(ctx context.Context) (SmallCachedValue, error) {
			return MakeCachedValue(i), nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	return FailoverByRunner{
		C: c,
		D: be.(interface {
			Delete(ctx context.Context, key string) error
		}),
		Cardinality: cardinality,
		Keys:        keys,
		WriteKeys:   writeKeys,
	}, fmt.Sprintf("FailoverByRunner(%T)", be)
}

// Run iterates over the cache.
func (r FailoverByRunner) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.Cardinality
		k := r.Keys[i]

		w++
		if w == writeEvery {
			w = 0

			k = r.WriteKeys[i]

			v, err := r.C.Get(ctx, k, func(ctx context.Context) (SmallCachedValue, error) {
				return MakeCachedValue(i), nil
			})
			if err != nil || v.I != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = r.D.Delete(ctx, k); err != nil && !errors.Is(err, cache.ErrNotFound) {
				b.Fatalf("err: %v, key: %s", err, k)
			}

			continue
		}

		v, err := r.C.Get(ctx, k, func(ctx context.Context) (SmallCachedValue, error) {
			panic("builder function should not be invoked while reading")
		})
		if err != nil || v.I != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}

func init() {
	Failovers = append(Failovers,
		FailoverByRunner{F: func() cache.ReadWriterBy[string, SmallCachedValue] {
			return cache.NewShardedMapBy[string, SmallCachedValue]()
		}},
		FailoverByRunner{F: func() cache.ReadWriterBy[string, SmallCachedValue] {
			return cache.NewSyncMapBy[string, SmallCachedValue]()
		}},
		FailoverOf{F: func() cache.ReadWriterOf[SmallCachedValue] {
			return cache.NewShardedMapOf[SmallCachedValue]()
		}},
	)

	ReadWriters = append(ReadWriters,
		ReadWriterByRunner{F: func() cache.ReadWriterBy[string, SmallCachedValue] {
			return cache.NewShardedMapBy[string, SmallCachedValue](func(cfg *cache.ConfigBy[string]) {
				cfg.CountSoftLimit = 1e10
				cfg.EvictionStrategy = cache.EvictLeastRecentlyUsed
			})
		}, Name: "ShardedMapBy/LRU"},
		ReadWriterByRunner{F: func() cache.ReadWriterBy[string, SmallCachedValue] {
			return cache.NewShardedMapBy[string, SmallCachedValue]()
		}},
		ReadWriterByRunner{F: func() cache.ReadWriterBy[string, SmallCachedValue] {
			return cache.NewSyncMapBy[string, SmallCachedValue]()
		}},
	)
}
