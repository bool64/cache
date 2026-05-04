//go:build go1.18
// +build go1.18

// Package filecache provides a local filesystem-backed implementation of
// cache.ReadWriterOf[blob.Entry] with persistent snapshots and immutable blob files.
package filecache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
)

const (
	indexFileName = "index.gob"
	dataDirName   = "data"
	fileExt       = ".blob"
)

var (
	_ cache.ReadWriterBy[string, blob.Entry]     = (*Storage[string])(nil)
	_ cache.WriteAndReaderBy[string, blob.Entry] = (*Storage[string])(nil)
)

// Storage is a local filesystem-backed blob storage.
type Storage[K comparable] struct {
	dir     string
	dataDir string

	index *cache.ShardedMapBy[K, storedEntry]
	log   cache.Logger
	split func(version string) []string

	mu          sync.Mutex
	openedFiles map[string]*openedFile

	openedWait sync.WaitGroup
	closeOnce  sync.Once
}

type storedEntry struct {
	Meta    blob.Meta
	Version string
}

type openedFile struct {
	refs int64
	dead bool
}

type blobInstance struct {
	open func() (io.ReadCloser, error)
	meta blob.Meta
}

func (b *blobInstance) Meta() blob.Meta {
	return b.meta
}

func (b *blobInstance) Open() (io.ReadCloser, error) {
	return b.open()
}

// NewStorage creates a local filesystem-backed blob storage.
func NewStorage[K comparable](path string, options ...func(cfg *Config[K])) (*Storage[K], error) {
	cfg := Config[K]{}

	for _, option := range options {
		option(&cfg)
	}

	if cfg.IndexPolicy.Name == "" {
		cfg.IndexPolicy.Name = "filecache:" + filepath.Base(path)
	}

	if cfg.IndexPolicy.TimeToLive == 0 {
		cfg.IndexPolicy.TimeToLive = cache.UnlimitedTTL
	}

	if cfg.SplitPath == nil {
		cfg.SplitPath = PrefixSplit(2, 2)
	}

	s := &Storage[K]{
		dir:         path,
		dataDir:     filepath.Join(path, dataDirName),
		log:         cfg.IndexPolicy.Logger,
		split:       cfg.SplitPath,
		openedFiles: make(map[string]*openedFile),
	}

	if err := os.MkdirAll(s.dataDir, 0o750); err != nil {
		return nil, err
	}

	s.index = cache.NewShardedMapBy[K, storedEntry](
		cache.WithPolicyBy[K, storedEntry](cfg.IndexPolicy),
		func(indexCfg *cache.ConfigBy[K, storedEntry]) {
			indexCfg.ShardFunc = cfg.IndexShardFunc
			indexCfg.OnDeleteBy = func(_ K, entry storedEntry) {
				if rmErr := s.removeVersion(entry.Version); rmErr != nil && s.log != nil {
					s.log.Error(context.Background(), "failed to delete blob file after index removal",
						"error", rmErr,
						"version", entry.Version,
						"path", s.pathForVersion(entry.Version),
						"name", cfg.IndexPolicy.Name,
					)
				}
			}
		},
	)

	if err := s.restoreIndex(); err != nil {
		s.index = nil

		return nil, err
	}

	return s, nil
}

// Read reads a blob entry by key.
func (s *Storage[K]) Read(ctx context.Context, key K) (blob.Entry, error) {
	v, err := s.index.Read(ctx, key)
	if err != nil {
		return nil, err
	}

	b := blobInstance{
		meta: v.Meta,
		open: func() (io.ReadCloser, error) {
			return s.openVersion(v.Version)
		},
	}

	return &b, err
}

// Write materializes a blob entry into local storage and updates the index.
func (s *Storage[K]) Write(ctx context.Context, key K, entry blob.Entry) error {
	_, err := s.WriteAndRead(ctx, key, entry)

	return err
}

// WriteAndRead materializes a blob entry into local storage, updates the index, and returns the stored entry.
func (s *Storage[K]) WriteAndRead(ctx context.Context, key K, entry blob.Entry) (_ blob.Entry, err error) {
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		if clErr := rc.Close(); clErr != nil && err == nil {
			err = clErr
		}
	}()

	version, err := newVersion()
	if err != nil {
		return nil, err
	}

	finalPath := s.pathForVersion(version)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(finalPath), version+".tmp-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()

	defer func() {
		if tmp != nil {
			if clErr := tmp.Close(); clErr != nil && err == nil {
				err = clErr
			}
		}

		if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
	}()

	if _, err := io.Copy(tmp, rc); err != nil {
		return nil, err
	}

	if err := tmp.Close(); err != nil {
		return nil, err
	}
	tmp = nil

	if err := os.Rename(tmpName, finalPath); err != nil {
		return nil, err
	}

	newEntry := storedEntry{
		Meta:    entry.Meta(),
		Version: version,
	}

	oldEntry, oldEntryExists := s.currentEntry(ctx, key)

	if err := s.index.Write(ctx, key, newEntry); err != nil {
		s.markDead(version)

		return nil, err
	}

	if oldEntryExists {
		s.markDead(oldEntry.Version)
	}

	b := blobInstance{
		meta: newEntry.Meta,
		open: func() (io.ReadCloser, error) {
			return s.openVersion(newEntry.Version)
		},
	}

	return &b, nil
}

// Delete deletes a blob entry by key.
func (s *Storage[K]) Delete(ctx context.Context, key K) error {
	return s.index.Delete(ctx, key)
}

// Close dumps the in-memory index and stops background jobs.
func (s *Storage[K]) Close() error {
	var err error

	s.closeOnce.Do(func() {
		err = s.dumpIndex()
		s.index = nil
		s.openedWait.Wait()
	})

	return err
}

func (s *Storage[K]) currentEntry(ctx context.Context, key K) (storedEntry, bool) {
	entry, err := s.index.Read(ctx, key)
	if err == nil {
		return entry, true
	}

	var errExpired cache.ErrWithExpiredItemOf[storedEntry]
	if errors.As(err, &errExpired) {
		return errExpired.Value(), true
	}

	return storedEntry{}, false
}

func (s *Storage[K]) openVersion(version string) (io.ReadCloser, error) {
	s.acquireVersion(version)

	//nolint:gosec // Path is derived from internal versioned storage layout, not external input.
	f, err := os.Open(s.pathForVersion(version))
	if err != nil {
		_ = s.releaseVersion(version)

		return nil, err
	}

	return &trackedFile{
		File: f,
		release: func() error {
			return s.releaseVersion(version)
		},
	}, nil
}

func (s *Storage[K]) releaseVersion(version string) error {
	defer s.openedWait.Done()

	s.mu.Lock()
	rt := s.openedFiles[version]
	if rt != nil {
		rt.refs -= 1
	}
	s.mu.Unlock()

	if rt == nil {
		return nil
	}

	if rt.refs == 0 && rt.dead {
		err := os.Remove(s.pathForVersion(version))

		s.mu.Lock()
		delete(s.openedFiles, version)
		s.mu.Unlock()

		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func (s *Storage[K]) acquireVersion(version string) *openedFile {
	s.openedWait.Add(1)

	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.openedFiles[version]
	if rt != nil {
		rt.refs += 1

		return rt
	}

	rt = &openedFile{
		refs: 1,
	}
	s.openedFiles[version] = rt

	return rt
}

func (s *Storage[K]) markDead(version string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.openedFiles[version]

	if rt == nil {
		return false
	}

	rt.dead = true

	return true
}

func (s *Storage[K]) removeVersion(version string) error {
	if s.markDead(version) {
		return nil
	}

	if err := os.Remove(s.pathForVersion(version)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (s *Storage[K]) restoreIndex() (err error) {
	f, err := os.Open(filepath.Join(s.dir, indexFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}
	defer func() {
		if clErr := f.Close(); clErr != nil && err == nil {
			err = clErr
		}
	}()

	_, err = s.index.Restore(f)
	return err
}

func (s *Storage[K]) dumpIndex() (err error) {
	tmp, err := os.CreateTemp(s.dir, indexFileName+".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	defer func() {
		if tmp != nil {
			if clErr := tmp.Close(); clErr != nil && err == nil {
				err = clErr
			}
		}

		if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && err == nil {
			err = rmErr
		}
	}()

	_, err = s.index.Dump(tmp)
	if err != nil {
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}
	tmp = nil

	return os.Rename(tmpName, filepath.Join(s.dir, indexFileName))
}

func (s *Storage[K]) pathForVersion(version string) string {
	path := s.dataDir

	if s.split != nil {
		for _, segment := range s.split(version) {
			if segment == "" {
				continue
			}

			path = filepath.Join(path, segment)
		}
	}

	return filepath.Join(path, version+fileExt)
}

func newVersion() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%s", hex.EncodeToString(buf[:]), strconv.FormatInt(time.Now().UnixNano(), 36)), nil
}

type trackedFile struct {
	*os.File

	once    sync.Once
	release func() error
	relErr  error
}

func (t *trackedFile) Close() error {
	err := t.File.Close()
	t.once.Do(func() {
		if t.release != nil {
			t.relErr = t.release()
		}
	})

	return errors.Join(err, t.relErr)
}
