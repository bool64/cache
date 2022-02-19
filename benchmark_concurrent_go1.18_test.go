//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/bool64/cache"
)

func init() {
	dflt = append(dflt, failoverG{f: func() cache.ReadWriterOf[smallCachedValue] {
		return cache.NewShardedMapOf[smallCachedValue]()
	}})

	all = append(all,
		backendG{f: func() cache.ReadWriterOf[smallCachedValue] {
			return cache.NewShardedMapOf[smallCachedValue]()
		}},

		shardedMapOfBaseline{},
	)
}

// failoverG is a benchmark runner.
type failoverG struct {
	f func() cache.ReadWriterOf[smallCachedValue]

	c           *cache.FailoverOf[smallCachedValue]
	d           cache.Deleter
	cardinality int
}

func (cl failoverG) make(b *testing.B, cardinality int) (cacheLoader, string) {
	b.Helper()

	be := cl.f()
	ctx := context.Background()
	c := cache.NewFailoverOf[smallCachedValue](cache.FailoverConfigOf[smallCachedValue]{
		Backend: be,
	}.Use)
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		_, err := c.Get(ctx, buf, func(ctx context.Context) (smallCachedValue, error) {
			return makeCachedValue(i), nil
		})
		if err != nil {
			b.Fail()
		}
	}

	return failoverG{
		c:           c,
		d:           be.(cache.Deleter),
		cardinality: cardinality,
	}, fmt.Sprintf("failover(%T)", be)
}

func (cl failoverG) run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			v, err := cl.c.Get(ctx, buf, func(ctx context.Context) (smallCachedValue, error) {
				return makeCachedValue(i), nil
			})

			if err != nil || v.i != i {
				b.Fatalf("err: %v, val: %v", err, v)
			}

			if err = cl.d.Delete(ctx, buf); err != nil && err != cache.ErrNotFound {
				b.Fatalf("err: %v, key: %s", err, string(buf))
			}

			continue
		}

		v, err := cl.c.Get(ctx, buf, func(ctx context.Context) (smallCachedValue, error) {
			panic("builder function should not be invoked while reading")
		})

		if err != nil || v.i != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}

type backendG struct {
	f func() cache.ReadWriterOf[smallCachedValue]

	be          cache.ReadWriterOf[smallCachedValue]
	d           cache.Deleter
	cardinality int
}

func (cl backendG) make(b *testing.B, cardinality int) (cacheLoader, string) {
	b.Helper()

	be := cl.f()
	ctx := context.Background()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		err := be.Write(ctx, buf, makeCachedValue(i))
		if err != nil {
			b.Fail()
		}
	}

	return backendG{
		be:          be,
		d:           be.(cache.Deleter),
		cardinality: cardinality,
	}, fmt.Sprintf("%T", be)
}

func (cl backendG) run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	ctx := context.Background()
	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			err := cl.be.Write(ctx, buf, makeCachedValue(i))
			if err != nil {
				b.Fatalf("err: %v", err)
			}

			if err = cl.d.Delete(ctx, buf); err != nil && err != cache.ErrNotFound {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, err := cl.be.Read(ctx, buf)

		if err != nil || v.i != i {
			b.Fatalf("err: %v, val: %v", err, v)
		}
	}
}
