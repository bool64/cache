package cache_test

import (
	"context"
	"testing"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

func TestIsSkipCache(t *testing.T) {
	ctx := context.Background()

	assert.True(t, cache.SkipRead(cache.WithSkipRead(ctx)))
	assert.False(t, cache.SkipRead(ctx))
}
