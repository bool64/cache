package cache_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

func ExampleNewInvalidationIndex() {
	// Invalidation index maintains lists of keys in multiple cache instances with shared labels.
	// For example, when you were building cache value, you used resources that can change in the future.
	// You can add a resource label to the new cache key (separate label for each resource), so that later,
	// when resource is changed, invalidation index can be asked to drop cached entries associated
	// with respective label.
	i := cache.NewInvalidationIndex()

	cache1 := cache.NewShardedMap()
	cache2 := cache.NewShardedMap()

	// Each cache instance, that is subject for invalidation needs to be added with unique name.
	i.AddCache("one", cache1)
	i.AddCache("two", cache2)

	ctx := context.Background()
	_ = cache1.Write(ctx, []byte("keyA"), "A1")
	_ = cache1.Write(ctx, []byte("keyB"), "B1")

	_ = cache2.Write(ctx, []byte("keyA"), "A2")
	_ = cache2.Write(ctx, []byte("keyB"), "B2")

	// Labeling keyA in both caches.
	i.AddLabels("one", []byte("keyA"), "A")
	i.AddLabels("two", []byte("keyA"), "A")

	// Labeling keyA only in one cache.
	i.AddLabels("one", []byte("keyB"), "B")

	// Invalidation will delete keyA in both cache one and two.
	n, _ := i.InvalidateByLabels(ctx, "A")
	fmt.Println("Keys deleted for A:", n)

	// Invalidation will not affect keyB in cache two, but will delete in cache one.
	n, _ = i.InvalidateByLabels(ctx, "B")
	fmt.Println("Keys deleted for B:", n)

	// Output:
	// Keys deleted for A: 2
	// Keys deleted for B: 1
}

func TestInvalidator_Invalidate(t *testing.T) {
	cache1 := cache.NewShardedMap()
	cache2 := cache.NewShardedMap()
	ctx := context.Background()

	i := &cache.Invalidator{}
	err := i.Invalidate(ctx)
	assert.Error(t, err) // nothing to invalidate

	i.Callbacks = append(i.Callbacks, cache1.ExpireAll, cache2.ExpireAll)

	assert.NoError(t, cache1.Write(ctx, []byte("key"), 1))
	assert.NoError(t, cache2.Write(ctx, []byte("key"), 2))

	val, err := cache1.Read(ctx, []byte("key"))
	assert.NoError(t, err)
	assert.Equal(t, 1, val)

	val, err = cache2.Read(ctx, []byte("key"))
	assert.NoError(t, err)
	assert.Equal(t, 2, val)

	err = i.Invalidate(ctx)
	assert.NoError(t, err)
	time.Sleep(time.Millisecond)

	_, err = cache1.Read(ctx, []byte("key"))
	assert.True(t, errors.Is(err, cache.ErrExpired))

	_, err = cache2.Read(ctx, []byte("key"))
	assert.True(t, errors.Is(err, cache.ErrExpired))

	err = i.Invalidate(ctx)
	assert.Error(t, err) // already invalidated
}

type deleterFunc func(ctx context.Context, key []byte) error

func (d deleterFunc) Delete(ctx context.Context, key []byte) error {
	return d(ctx, key)
}

func TestInvalidationIndex_InvalidateGroups_delete_fails(t *testing.T) {
	f := func(ctx context.Context, key []byte) error {
		if string(key) == "three" {
			return errors.New("failed for three")
		}

		return nil
	}
	d := deleterFunc(func(ctx context.Context, key []byte) error {
		return f(ctx, key)
	})

	ii := cache.NewInvalidationIndex(d)

	ii.AddInvalidationLabels([]byte("one"), "numbers", "len3")
	ii.AddInvalidationLabels([]byte("two"), "numbers", "len3")
	ii.AddInvalidationLabels([]byte("three"), "numbers", "len5")
	ii.AddInvalidationLabels([]byte("four"), "numbers", "len4")
	ii.AddInvalidationLabels([]byte("five"), "numbers", "len4")

	n, err := ii.InvalidateByLabels(context.Background(), "len3", "len5", "numbers")

	assert.Equal(t, 2, n)
	assert.EqualError(t, err, "failed for three")

	n, err = ii.InvalidateByLabels(context.Background(), "len3", "len5", "numbers")

	assert.Equal(t, 0, n)
	assert.EqualError(t, err, "failed for three")

	f = func(ctx context.Context, key []byte) error {
		return nil
	}

	n, err = ii.InvalidateByLabels(context.Background(), "len3", "len5", "numbers")

	assert.Equal(t, 3, n)
	assert.NoError(t, err)
}
