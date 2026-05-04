//go:build go1.18
// +build go1.18

package filecache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_persistedIndex(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{
		Name: "a.txt",
	})))
	require.NoError(t, s.Close())

	s, err = NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	entry, err := s.Read(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "a.txt", entry.Meta().Name)

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "value", string(b))
}

func TestStorage_rewriteDeletesOldVersionAfterLastClose(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	key := "same"

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("v1"), blob.Meta{Name: "v1.txt"})))

	entry, err := s.Read(ctx, key)
	require.NoError(t, err)

	rc, err := entry.Open()
	require.NoError(t, err)

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("v2"), blob.Meta{Name: "v2.txt"})))
	require.NoError(t, rc.Close())

	newEntry, err := s.Read(ctx, key)
	require.NoError(t, err)

	newRC, err := newEntry.Open()
	require.NoError(t, err)
	defer newRC.Close()

	b, err := io.ReadAll(newRC)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(b))
}

func TestStorage_deleteRemovesFile(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	key := "delete-me"

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("gone"), blob.Meta{})))
	entry, err := s.Read(ctx, key)
	require.NoError(t, err)

	require.NoError(t, s.Delete(ctx, key))

	_, err = entry.Open()
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStorage_customPathSplit(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir, Config[string]{
		SplitPath: PrefixSplit(1),
	}.Use)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{})))

	entry, ok := s.currentEntry(ctx, "k1")
	require.True(t, ok)

	expectedPath := filepath.Join(dir, dataDirName, entry.Version[:1], entry.Version+fileExt)
	_, err = os.Stat(expectedPath)
	require.NoError(t, err)
}

func TestStorage_storedBytesSoftLimit(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir, Config[string]{
		IndexPolicy: cache.Policy{
			TimeToLive:               cache.UnlimitedTTL,
			DeleteExpiredJobInterval: time.Millisecond,
			EvictFraction:            0.5,
		},
		RetentionPolicy: blob.RetentionPolicy{
			StoredBytesSoftLimit: 10,
		},
	}.Use)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("12345"), blob.Meta{})))
	require.NoError(t, s.Write(ctx, "k2", blob.FromReader(bytes.NewBufferString("12345"), blob.Meta{})))
	require.NoError(t, s.Write(ctx, "k3", blob.FromReader(bytes.NewBufferString("12345"), blob.Meta{})))

	require.Eventually(t, func() bool {
		total := atomic.LoadInt64(&s.bytes)
		total = max(total, 0)

		return s.index.Len() <= 2 && total <= 10
	}, time.Second, 10*time.Millisecond)
}

func TestStorage_rewriteUpdatesStoredBytes(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "same", blob.FromReader(bytes.NewBufferString("12345"), blob.Meta{})))
	assert.Equal(t, int64(5), atomic.LoadInt64(&s.bytes))

	require.NoError(t, s.Write(ctx, "same", blob.FromReader(bytes.NewBufferString("12"), blob.Meta{})))
	assert.Equal(t, int64(2), atomic.LoadInt64(&s.bytes))
}
