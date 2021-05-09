package cache_test

import (
	"context"
	"testing"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

func TestNoOp_Read(t *testing.T) {
	v, err := cache.NoOp{}.Read(context.Background(), []byte("foo"))
	assert.Nil(t, v)
	assert.EqualError(t, err, cache.ErrNotFound.Error())
}

func TestNoOp_Write(t *testing.T) {
	err := cache.NoOp{}.Write(context.Background(), []byte("foo"), 123)
	assert.NoError(t, err)

	v, err := cache.NoOp{}.Read(context.Background(), []byte("foo"))
	assert.Nil(t, v)
	assert.EqualError(t, err, cache.ErrNotFound.Error())
}

func TestNoOp_Delete(t *testing.T) {
	err := cache.NoOp{}.Delete(context.Background(), []byte("foo"))
	assert.EqualError(t, err, cache.ErrNotFound.Error())
}
