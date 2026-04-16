//go:build go1.18
// +build go1.18

// Package filecache provides a local filesystem-backed implementation of
// cache.ReadWriterOf[blob.Entry] with persistent snapshots and immutable blob files.
package filecache

import (
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
)

const (
	indexFileName = "index.gob"
	dataDirName   = "data"
	fileExt       = ".blob"
)

// Storage is a local filesystem-backed blob storage.
type Storage struct {
	dir     string
	dataDir string

	index *cache.ShardedMapOf[blob.Entry]

	mu      sync.Mutex
	runtime map[string]*runtimeFile

	closeOnce sync.Once
}

type storedEntry struct {
	storage *Storage
	meta    blob.Meta
	version string
}

type persistedEntry struct {
	Key      []byte
	Meta     blob.Meta
	Version  string
	ExpireAt time.Time
}

type runtimeFile struct {
	path string
	refs int64
	dead bool
}

// NewStorage creates a local filesystem-backed blob storage.
func NewStorage(path string) (*Storage, error) {
	s := &Storage{
		dir:     path,
		dataDir: filepath.Join(path, dataDirName),
		runtime: make(map[string]*runtimeFile),
	}

	if err := os.MkdirAll(s.dataDir, 0o750); err != nil {
		return nil, err
	}

	s.index = cache.NewShardedMapOf[blob.Entry](cache.Config{
		Name:               "filecache:" + filepath.Base(path),
		ExpirationJitter:   -1,
		EvictionStrategy:   cache.EvictMostExpired,
		TimeToLive:         cache.UnlimitedTTL,
		DeleteExpiredAfter: 24 * time.Hour,
		OnDelete: func(_ []byte, value interface{}) {
			if entry, ok := value.(blob.Entry); ok {
				s.markDead(entry)
			}
		},
	}.Use)

	if err := s.restoreIndex(); err != nil {
		s.index = nil

		return nil, err
	}

	if err := s.reconcileFiles(); err != nil {
		s.index = nil

		return nil, err
	}

	return s, nil
}

// Read reads a blob entry by key.
func (s *Storage) Read(ctx context.Context, key []byte) (blob.Entry, error) {
	return s.index.Read(ctx, key)
}

// Write materializes a blob entry into local storage and updates the index.
func (s *Storage) Write(ctx context.Context, key []byte, entry blob.Entry) error {
	_, err := s.WriteAndRead(ctx, key, entry)

	return err
}

// WriteAndRead materializes a blob entry into local storage, updates the index, and returns the stored entry.
func (s *Storage) WriteAndRead(ctx context.Context, key []byte, entry blob.Entry) (blob.Entry, error) {
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}

	version, err := newVersion()
	if err != nil {
		_ = rc.Close()

		return nil, err
	}

	finalPath := s.pathForVersion(version)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
		_ = rc.Close()

		return nil, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(finalPath), version+".tmp-*")
	if err != nil {
		_ = rc.Close()

		return nil, err
	}

	tmpName := tmp.Name()
	cleanupTemp := func() {
		_ = os.Remove(tmpName)
	}

	if _, err := io.Copy(tmp, rc); err != nil {
		_ = tmp.Close()
		_ = rc.Close()

		cleanupTemp()

		return nil, err
	}

	if err := tmp.Close(); err != nil {
		_ = rc.Close()

		cleanupTemp()

		return nil, err
	}

	if err := rc.Close(); err != nil {
		cleanupTemp()

		return nil, err
	}

	if err := os.Rename(tmpName, finalPath); err != nil {
		cleanupTemp()

		return nil, err
	}

	newEntry := &storedEntry{
		storage: s,
		meta:    entry.Meta(),
		version: version,
	}

	s.ensureRuntime(version, finalPath)

	oldEntry, _ := s.currentEntry(ctx, key)

	if err := s.index.Write(ctx, key, newEntry); err != nil {
		s.markDead(newEntry)

		return nil, err
	}

	if oldEntry != nil {
		s.markDead(oldEntry)
	}

	return newEntry, nil
}

// Delete deletes a blob entry by key.
func (s *Storage) Delete(ctx context.Context, key []byte) error {
	return s.index.Delete(ctx, key)
}

// Close dumps the in-memory index and stops background jobs.
func (s *Storage) Close() error {
	var err error

	s.closeOnce.Do(func() {
		err = s.dumpIndex()
		if reconcileErr := s.reconcileFiles(); err == nil {
			err = reconcileErr
		}

		s.index = nil
	})

	return err
}

func (e *storedEntry) Meta() blob.Meta {
	return e.meta
}

func (e *storedEntry) Open() (io.ReadCloser, error) {
	return e.storage.openVersion(e.version)
}

func (s *Storage) currentEntry(ctx context.Context, key []byte) (blob.Entry, bool) {
	entry, err := s.index.Read(ctx, key)
	if err == nil {
		return entry, true
	}

	var errExpired cache.ErrWithExpiredItemOf[blob.Entry]
	if errors.As(err, &errExpired) {
		return errExpired.Value(), true
	}

	return nil, false
}

func (s *Storage) openVersion(version string) (io.ReadCloser, error) {
	path := s.pathForVersion(version)
	rt := s.ensureRuntime(version, path)
	atomic.AddInt64(&rt.refs, 1)

	//nolint:gosec // Path is derived from internal versioned storage layout, not external input.
	f, err := os.Open(path)
	if err != nil {
		atomic.AddInt64(&rt.refs, -1)

		return nil, err
	}

	return &trackedFile{
		File: f,
		release: func() {
			s.releaseVersion(version)
		},
	}, nil
}

func (s *Storage) releaseVersion(version string) {
	s.mu.Lock()
	rt := s.runtime[version]
	s.mu.Unlock()

	if rt == nil {
		return
	}

	if atomic.AddInt64(&rt.refs, -1) == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()

		rt = s.runtime[version]
		if rt == nil || atomic.LoadInt64(&rt.refs) != 0 || !rt.dead {
			return
		}

		_ = os.Remove(rt.path)

		delete(s.runtime, version)
	}
}

func (s *Storage) ensureRuntime(version, path string) *runtimeFile {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rt, ok := s.runtime[version]; ok {
		if rt.path == "" {
			rt.path = path
		}

		return rt
	}

	rt := &runtimeFile{path: path}
	s.runtime[version] = rt

	return rt
}

func (s *Storage) markDead(entry blob.Entry) {
	se, ok := entry.(*storedEntry)
	if !ok || se == nil {
		return
	}

	path := s.pathForVersion(se.version)

	s.mu.Lock()
	rt := s.runtime[se.version]

	if rt == nil {
		rt = &runtimeFile{path: path}
		s.runtime[se.version] = rt
	}

	rt.dead = true
	refs := atomic.LoadInt64(&rt.refs)
	s.mu.Unlock()

	if refs == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()

		rt = s.runtime[se.version]
		if rt == nil || atomic.LoadInt64(&rt.refs) != 0 || !rt.dead {
			return
		}

		_ = os.Remove(rt.path)

		delete(s.runtime, se.version)
	}
}

func (s *Storage) restoreIndex() error {
	f, err := os.Open(filepath.Join(s.dir, indexFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}
	defer f.Close()

	var entries []persistedEntry
	if err := gob.NewDecoder(f).Decode(&entries); err != nil {
		return err
	}

	now := time.Now()

	for _, pe := range entries {
		if _, err := os.Stat(s.pathForVersion(pe.Version)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return err
		}

		ctx := context.Background()
		if pe.ExpireAt.UnixNano() != 0 {
			ctx = cache.WithTTL(ctx, pe.ExpireAt.Sub(now), false)
		}

		entry := &storedEntry{
			storage: s,
			meta:    pe.Meta,
			version: pe.Version,
		}

		s.ensureRuntime(pe.Version, s.pathForVersion(pe.Version))

		if err := s.index.Write(ctx, pe.Key, entry); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) dumpIndex() error {
	entries := make([]persistedEntry, 0)

	_, err := s.index.Walk(func(entry cache.EntryOf[blob.Entry]) error {
		se, ok := entry.Value().(*storedEntry)
		if !ok || se == nil {
			return fmt.Errorf("unexpected entry type %T", entry.Value())
		}

		key := append([]byte(nil), entry.Key()...)
		entries = append(entries, persistedEntry{
			Key:      key,
			Meta:     se.meta,
			Version:  se.version,
			ExpireAt: entry.ExpireAt(),
		})

		return nil
	})
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, indexFileName+".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := gob.NewEncoder(tmp).Encode(entries); err != nil {
		_ = tmp.Close()

		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, filepath.Join(s.dir, indexFileName))
}

func (s *Storage) reconcileFiles() error {
	referenced := make(map[string]struct{})
	busyPaths := make(map[string]struct{})

	_, err := s.index.Walk(func(entry cache.EntryOf[blob.Entry]) error {
		se, ok := entry.Value().(*storedEntry)
		if !ok || se == nil {
			return fmt.Errorf("unexpected entry type %T", entry.Value())
		}

		referenced[s.pathForVersion(se.version)] = struct{}{}

		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	for _, rt := range s.runtime {
		if atomic.LoadInt64(&rt.refs) > 0 {
			busyPaths[rt.path] = struct{}{}
		}
	}
	s.mu.Unlock()

	return filepath.Walk(s.dataDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != fileExt && filepath.Ext(path) != ".tmp" {
			return nil
		}

		if filepath.Ext(path) == ".tmp" {
			//nolint:gosec // Reconciliation only removes files under application-owned cache storage.
			return os.Remove(path)
		}

		if _, busy := busyPaths[path]; busy {
			return nil
		}

		if _, ok := referenced[path]; ok {
			return nil
		}

		//nolint:gosec // Reconciliation only removes files under application-owned cache storage.
		return os.Remove(path)
	})
}

func (s *Storage) pathForVersion(version string) string {
	path := s.dataDir
	if len(version) >= 2 {
		path = filepath.Join(path, version[:2])
	}

	if len(version) >= 4 {
		path = filepath.Join(path, version[2:4])
	}

	return filepath.Join(path, version+fileExt)
}

func newVersion() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf("%d%s", time.Now().UTC().UnixNano(), hex.EncodeToString(buf[:])), nil
}

type trackedFile struct {
	*os.File

	once    sync.Once
	release func()
}

func (t *trackedFile) Close() error {
	err := t.File.Close()
	t.once.Do(func() {
		if t.release != nil {
			t.release()
		}
	})

	return err
}
