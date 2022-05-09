package cache_test

import (
	"testing"

	"github.com/bool64/cache"
	"github.com/stretchr/testify/assert"
)

func TestHTTPTransfer_CachesCount(t *testing.T) {
	tr := cache.HTTPTransfer{}
	assert.Equal(t, 0, tr.CachesCount())

	tr.AddCache("test", cache.NewShardedMap())
	assert.Equal(t, 1, tr.CachesCount())

	tr.AddCache("test2", cache.NewShardedMap())
	assert.Equal(t, 2, tr.CachesCount())
}
