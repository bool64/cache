package cache_test

import (
	"context"
	"testing"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

func TestFailoverOf_Get(t *testing.T) {
	f := cache.NewFailoverOf[string]()

	s, err := f.Get(context.Background(), []byte("aaaa"), func(ctx context.Context) (string, error) {
		return "bbb", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "bbb", s)
}
