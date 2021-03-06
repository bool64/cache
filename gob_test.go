package cache_test

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strconv"
	"testing"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SomeEntity struct {
	Parent           *SomeEntity
	SomeField        string
	SomeSlice        []int
	SomeRecursiveMap map[string]SomeEntity
	unexported       string
}

func ExampleGobRegister_transfer_cache() {
	// Registering cached type to gob.
	cache.GobRegister(SomeEntity{})

	c1 := cache.NewShardedMap()
	c2 := cache.NewShardedMap()
	ctx := context.Background()

	_ = c1.Write(ctx, []byte("key1"), SomeEntity{
		SomeField:  "foo",
		SomeSlice:  []int{1, 2, 3},
		unexported: "will be lost in transfer",
	})

	w := bytes.NewBuffer(nil)

	// Transferring data from c1 to c2.
	_, _ = c1.Dump(w)
	_, _ = c2.Restore(w)

	v, _ := c2.Read(ctx, []byte("key1"))

	fmt.Println(v.(SomeEntity).SomeField)

	// Output: foo
}

func Test_DumpRestore(t *testing.T) {
	cache.GobTypesHashReset()
	cache.GobRegister(SomeEntity{})

	assert.Equal(t, uint64(0xf6c7853229d11f18), cache.GobTypesHash())

	b1 := backends()
	b2 := backends()

	for i, be := range b1 {
		i := i
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			c1, ok := b1[i].(interface {
				cache.WalkDumpRestorer
				cache.ReadWriter
			})
			if !ok {
				return
			}

			c2, ok := b2[i].(interface {
				cache.WalkDumpRestorer
				cache.ReadWriter
			})
			if !ok {
				return
			}

			ctx := context.Background()

			require.NoError(t, c1.Write(ctx, []byte("key1"), SomeEntity{
				SomeField:  "foo",
				SomeSlice:  []int{1, 2, 3},
				unexported: "will be lost in transfer",
			}))
			require.NoError(t, c1.Write(ctx, []byte("key2"), SomeEntity{SomeField: "bar"}))

			v, err := c1.Read(ctx, []byte("key1"))

			assert.NoError(t, err)
			assert.Equal(t, SomeEntity{
				SomeField:  "foo",
				SomeSlice:  []int{1, 2, 3},
				unexported: "will be lost in transfer",
			}, v)

			w := bytes.NewBuffer(nil)
			n, err := c1.Dump(w)

			require.NoError(t, err)
			assert.Equal(t, 2, n)

			n, err = c2.Restore(w)

			require.NoError(t, err)
			assert.Equal(t, 2, n)

			v, err = c2.Read(ctx, []byte("key1"))

			assert.NoError(t, err, "missing value for key1")
			assert.Equal(t, SomeEntity{SomeField: "foo", SomeSlice: []int{1, 2, 3}}, v)

			v, err = c2.Read(ctx, []byte("key2"))

			assert.NoError(t, err, "missing value for key2")
			assert.Equal(t, SomeEntity{SomeField: "bar"}, v)
		})
	}
}

func BenchmarkShardedMap_Dump(b *testing.B) {
	cache.GobRegister(SomeEntity{})

	c1 := cache.NewShardedMap()
	c2 := cache.NewShardedMap()
	ctx := context.Background()

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := c1.Write(ctx, []byte("key"+strconv.Itoa(i)), SomeEntity{SomeField: "foo"})
		if err != nil {
			assert.NoError(b, err)

			return
		}
	}

	w := bytes.NewBuffer(nil)
	n, err := c1.Dump(w)

	require.NoError(b, err)
	assert.Equal(b, b.N, n)

	n, err = c2.Restore(w)

	require.NoError(b, err)
	assert.Equal(b, b.N, n)

	runtime.GC()
}
