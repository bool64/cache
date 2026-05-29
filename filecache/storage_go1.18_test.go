//go:build go1.18
// +build go1.18

package filecache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
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
		if total < 0 {
			total = 0
		}

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

func TestStorage_walkReturnsStoredEntries(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := cache.WithTTL(context.Background(), time.Minute, false)
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{
		Name: "walk.txt",
	})))

	var seen cache.EntryBy[string, blob.Entry]

	n, err := s.Walk(func(entry cache.EntryBy[string, blob.Entry]) error {
		seen = entry

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NotNil(t, seen)

	assert.Equal(t, "k1", seen.Key())
	assert.Equal(t, "walk.txt", seen.Value().Meta().Name)
	assert.WithinDuration(t, time.Now().Add(time.Minute), seen.ExpireAt(), 5*time.Second)

	rc, err := seen.Value().Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "value", string(b))
}

func TestStorage_flushPersistsIndex(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{
		Name: "flush.txt",
	})))
	require.NoError(t, s.Flush())
	require.NoError(t, s.Close())

	reopened, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reopened.Close())
	}()

	entry, err := reopened.Read(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "flush.txt", entry.Meta().Name)
}

func TestStorage_closeRetriesAfterFlushFailure(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "k1", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{})))

	origDir := s.dir
	s.dir = filepath.Join(dir, "missing")

	err = s.Close()
	require.Error(t, err)

	s.dir = origDir
	require.NoError(t, s.Close())

	reopened, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reopened.Close())
	}()

	entry, err := reopened.Read(ctx, "k1")
	require.NoError(t, err)

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "value", string(b))
}

func TestStorage_repairDryRunReportsInvalidEntriesAndOrphans(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "valid", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{})))
	require.NoError(t, s.Write(ctx, "broken", blob.FromReader(bytes.NewBufferString("missing"), blob.Meta{})))

	broken, ok := s.currentEntry(ctx, "broken")
	require.True(t, ok)
	require.NoError(t, os.Remove(s.pathForVersion(broken.Version)))

	orphanPath := filepath.Join(dir, dataDirName, "orphan", "ghost"+fileExt)
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanPath), 0o750))
	require.NoError(t, os.WriteFile(orphanPath, []byte("orphan"), 0o600))

	var (
		invalidKeys  []string
		invalidPaths []string
		orphanPaths  []string
	)

	result := s.Repair(func(cfg *RepairConfig[string]) {
		cfg.DryRun = true
		cfg.OnInvalidEntry = func(key string, path string, removeErr error) {
			require.NoError(t, removeErr)

			invalidKeys = append(invalidKeys, key)
			invalidPaths = append(invalidPaths, path)
		}
		cfg.OnOrphanFile = func(path string, removeErr error) {
			require.NoError(t, removeErr)

			orphanPaths = append(orphanPaths, path)
		}
	})

	assert.Equal(t, RepairResult{
		InvalidEntries: 1,
		OrphanFiles:    1,
	}, result)
	assert.Equal(t, []string{"broken"}, invalidKeys)
	assert.Equal(t, []string{s.pathForVersion(broken.Version)}, invalidPaths)
	assert.Equal(t, []string{orphanPath}, orphanPaths)

	_, err = s.Read(ctx, "broken")
	require.NoError(t, err)

	_, err = os.Stat(orphanPath)
	require.NoError(t, err)
}

func TestStorage_repairRemovesInvalidEntriesAndOrphans(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "broken", blob.FromReader(bytes.NewBufferString("missing"), blob.Meta{})))

	broken, ok := s.currentEntry(ctx, "broken")
	require.True(t, ok)
	require.NoError(t, os.Remove(s.pathForVersion(broken.Version)))

	orphanPath := filepath.Join(dir, dataDirName, "orphan", "ghost"+fileExt)
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanPath), 0o750))
	require.NoError(t, os.WriteFile(orphanPath, []byte("orphan"), 0o600))

	var (
		invalidKeys []string
		orphanPaths []string
	)

	result := s.Repair(func(cfg *RepairConfig[string]) {
		cfg.OnInvalidEntry = func(key string, path string, removeErr error) {
			require.NoError(t, removeErr)

			invalidKeys = append(invalidKeys, key)
		}
		cfg.OnOrphanFile = func(path string, removeErr error) {
			require.NoError(t, removeErr)

			orphanPaths = append(orphanPaths, path)
		}
	})

	assert.Equal(t, RepairResult{
		InvalidEntries: 1,
		OrphanFiles:    1,
		RemovedEntries: 1,
		RemovedFiles:   1,
	}, result)
	assert.Equal(t, []string{"broken"}, invalidKeys)
	assert.Equal(t, []string{orphanPath}, orphanPaths)

	_, err = s.Read(ctx, "broken")
	assert.ErrorIs(t, err, cache.ErrNotFound)

	_, err = os.Stat(orphanPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStorage_repairIgnoresPendingDeadFiles(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, s.Close())
	}()

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, "same", blob.FromReader(bytes.NewBufferString("value"), blob.Meta{})))

	entry, err := s.Read(ctx, "same")
	require.NoError(t, err)

	rc, err := entry.Open()
	require.NoError(t, err)

	require.NoError(t, s.Delete(ctx, "same"))

	result := s.Repair()
	assert.Equal(t, RepairResult{}, result)

	require.NoError(t, rc.Close())
}

func TestStorage_repairContinuesAfterOrphanRemoveFailure(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage[string](dir)
	require.NoError(t, err)

	orphanA := filepath.Join(dir, dataDirName, "orphan-a"+fileExt)
	orphanB := filepath.Join(dir, dataDirName, "nested", "orphan-b"+fileExt)

	defer func() {
		require.NoError(t, os.Chmod(filepath.Join(dir, dataDirName), 0o750))
		require.NoError(t, os.Chmod(filepath.Dir(orphanB), 0o750))
		require.NoError(t, s.Close())
	}()

	require.NoError(t, os.WriteFile(orphanA, []byte("a"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanB), 0o750))
	require.NoError(t, os.WriteFile(orphanB, []byte("b"), 0o600))
	require.NoError(t, os.Chmod(filepath.Dir(orphanB), 0o500))

	var (
		callbackPaths []string
		callbackErrs  int
		reportedErrs  int
	)

	result := s.Repair(func(cfg *RepairConfig[string]) {
		cfg.OnOrphanFile = func(path string, removeErr error) {
			callbackPaths = append(callbackPaths, path)

			if removeErr != nil {
				callbackErrs++
			}
		}
		cfg.OnError = func(err error) {
			reportedErrs++
		}
	})

	sort.Strings(callbackPaths)

	expectedPaths := []string{orphanA, orphanB}
	sort.Strings(expectedPaths)

	assert.Equal(t, RepairResult{
		OrphanFiles:  2,
		RemovedFiles: 1,
	}, result)
	assert.Equal(t, expectedPaths, callbackPaths)
	assert.Equal(t, 1, callbackErrs)
	assert.Equal(t, 1, reportedErrs)
}
