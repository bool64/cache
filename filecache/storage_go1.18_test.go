//go:build go1.18
// +build go1.18

package filecache

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/bool64/cache/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_persistedIndex(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage(dir)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, []byte("k1"), blob.FromReader(bytes.NewBufferString("value"), blob.Meta{
		Name: "a.txt",
	})))
	require.NoError(t, s.Close())

	s, err = NewStorage(dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	entry, err := s.Read(ctx, []byte("k1"))
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

	s, err := NewStorage(dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	key := []byte("same")

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("v1"), blob.Meta{Name: "v1.txt"})))

	entry, err := s.Read(ctx, key)
	require.NoError(t, err)

	oldStored, ok := entry.(*storedEntry)
	require.True(t, ok)

	oldPath := s.pathForVersion(oldStored.version)

	rc, err := entry.Open()
	require.NoError(t, err)

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("v2"), blob.Meta{Name: "v2.txt"})))

	_, err = os.Stat(oldPath)
	require.NoError(t, err)

	require.NoError(t, rc.Close())

	_, err = os.Stat(oldPath)
	assert.ErrorIs(t, err, os.ErrNotExist)

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

	s, err := NewStorage(dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	key := []byte("delete-me")

	require.NoError(t, s.Write(ctx, key, blob.FromReader(bytes.NewBufferString("gone"), blob.Meta{})))
	entry, err := s.Read(ctx, key)
	require.NoError(t, err)

	se, ok := entry.(*storedEntry)
	require.True(t, ok)

	path := s.pathForVersion(se.version)

	require.NoError(t, s.Delete(ctx, key))

	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
