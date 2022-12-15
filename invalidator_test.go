package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

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
