//go:build go1.18
// +build go1.18

package filecache_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
	"github.com/bool64/cache/filecache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailoverOf_blobEntry_returnsStoredEntryForOneShotSource(t *testing.T) {
	dir := t.TempDir()

	st, err := filecache.NewStorage(dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, st.Close())
	}()

	f := cache.NewFailoverOf[blob.Entry](func(cfg *cache.FailoverConfigOf[blob.Entry]) {
		cfg.Backend = st
	})

	ctx := context.Background()

	entry, err := f.Get(ctx, []byte("photo:1"), func(ctx context.Context) (blob.Entry, error) {
		return blob.FromReader(bytes.NewBufferString("hello"), blob.Meta{Name: "photo.txt"}), nil
	})
	require.NoError(t, err)

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)

	assert.Equal(t, "hello", string(b))
	assert.Equal(t, "photo.txt", entry.Meta().Name)
}
