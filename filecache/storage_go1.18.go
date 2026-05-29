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
	"math"
	"os"
	"path/filepath"
	"strconv"
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

var (
	_     cache.ReadWriterBy[string, blob.Entry]     = (*Storage[string])(nil)
	_     cache.WriteAndReaderBy[string, blob.Entry] = (*Storage[string])(nil)
	_     cache.WalkerBy[string, blob.Entry]         = (*Storage[string])(nil)
	bgCtx                                            = context.Background()
)

// Storage is a local filesystem-backed blob storage.
type Storage[K comparable] struct {
	dir     string
	dataDir string

	index *cache.ShardedMapBy[K, storedEntry]
	log   cache.Logger
	split func(version string) []string
	limit uint64
	bytes int64

	closeMu     sync.Mutex
	closed      bool
	mu          sync.Mutex
	openedFiles map[string]*openedFile

	openedWait sync.WaitGroup
}

type storedEntry struct {
	Meta    blob.Meta
	Version string
}

type openedFile struct {
	refs int64
	dead bool
}

type invalidEntry[K comparable] struct {
	key     K
	version string
}

type invalidReport[K comparable] struct {
	key       K
	path      string
	removeErr error
}

type orphanReport struct {
	path      string
	removeErr error
}

type blobInstance struct {
	open func() (io.ReadCloser, error)
	meta blob.Meta
}

// RepairResult summarizes filecache reconciliation results.
type RepairResult struct {
	InvalidEntries int
	OrphanFiles    int
	RemovedEntries int
	RemovedFiles   int
}

// RepairConfig controls best-effort storage reconciliation.
type RepairConfig[K comparable] struct {
	DryRun bool

	OnInvalidEntry func(key K, path string, removeErr error)
	OnOrphanFile   func(path string, removeErr error)
	OnError        func(error)
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
		limit:       cfg.StoredBytesSoftLimit,
		openedFiles: make(map[string]*openedFile),
	}

	if err := os.MkdirAll(s.dataDir, 0o750); err != nil {
		return nil, err
	}

	s.index = cache.NewShardedMapBy[K, storedEntry](
		cache.WithPolicyBy[K, storedEntry](cfg.IndexPolicy),
		func(indexCfg *cache.ConfigBy[K, storedEntry]) {
			origEvictionNeeded := indexCfg.EvictionNeeded

			if s.limit > 0 {
				indexCfg.EvictionNeeded = func() bool {
					return s.storedBytesOverflow() || (origEvictionNeeded != nil && origEvictionNeeded())
				}
			}

			indexCfg.ShardFunc = cfg.IndexShardFunc
			indexCfg.OnDeleteBy = func(_ K, entry storedEntry) {
				s.addStoredBytes(-entry.Meta.Size)

				if rmErr := s.removeVersion(entry.Version); rmErr != nil && s.log != nil {
					s.log.Error(bgCtx, "failed to delete blob file after index removal",
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
			err = joinErr(err, rmErr)
		}
	}()

	size, err := io.Copy(tmp, rc)
	if err != nil {
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
	newEntry.Meta.Size = size

	oldEntry, oldEntryExists := s.currentEntry(ctx, key)

	if err := s.index.Write(ctx, key, newEntry); err != nil {
		_ = s.removeVersion(version)

		return nil, err
	}

	s.addStoredBytes(newEntry.Meta.Size)

	if oldEntryExists {
		s.addStoredBytes(-oldEntry.Meta.Size)

		if rmErr := s.removeVersion(oldEntry.Version); rmErr != nil && s.log != nil {
			s.log.Error(ctx, "failed to delete superseded blob file",
				"error", rmErr,
				"version", oldEntry.Version,
				"path", s.pathForVersion(oldEntry.Version),
			)
		}
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

// Walk traverses all entries in the storage, applying the provided callback function to each entry.
// Returns the number of entries processed and any error encountered during the traversal.
func (s *Storage[K]) Walk(cb func(entry cache.EntryBy[K, blob.Entry]) error) (int, error) {
	return s.index.Walk(func(e cache.EntryBy[K, storedEntry]) error {
		v := e.Value()

		b := blobInstance{
			meta: v.Meta,
			open: func() (io.ReadCloser, error) {
				return s.openVersion(v.Version)
			},
		}

		t := cache.TraitEntryBy[K, blob.Entry]{
			K: e.Key(),
			V: &b,
			E: e.ExpireAt().UnixNano(),
		}

		return cb(t)
	})
}

// Repair checks indexed entries against blob files and removes broken entries and orphaned files best-effort.
func (s *Storage[K]) Repair(options ...func(*RepairConfig[K])) RepairResult {
	cfg := RepairConfig[K]{}

	for _, option := range options {
		option(&cfg)
	}

	s.closeMu.Lock()
	if s.closed || s.index == nil {
		s.closeMu.Unlock()

		return RepairResult{}
	}

	liveVersions, invalidEntries := s.collectInvalidEntries(&cfg)

	result := RepairResult{
		InvalidEntries: len(invalidEntries),
	}

	invalidReports := s.repairInvalidEntries(&cfg, invalidEntries, &result)
	orphanPaths := s.collectOrphanPaths(&cfg, liveVersions)
	s.closeMu.Unlock()

	result.OrphanFiles = len(orphanPaths)

	orphanReports := s.repairOrphanFiles(&cfg, orphanPaths, &result)
	s.reportRepair(invalidReports, orphanReports, &cfg)

	return result
}

func (s *Storage[K]) collectInvalidEntries(cfg *RepairConfig[K]) (map[string]struct{}, []invalidEntry[K]) {
	liveVersions := make(map[string]struct{})
	invalidEntries := make([]invalidEntry[K], 0)

	_, _ = s.index.Walk(func(entry cache.EntryBy[K, storedEntry]) error {
		value := entry.Value()
		liveVersions[value.Version] = struct{}{}

		path := s.pathForVersion(value.Version)
		if _, err := os.Stat(path); err != nil {
			invalidEntries = append(invalidEntries, invalidEntry[K]{
				key:     entry.Key(),
				version: value.Version,
			})

			s.reportRepairError(cfg, err, os.ErrNotExist)
		}

		return nil
	})

	return liveVersions, invalidEntries
}

func (s *Storage[K]) repairInvalidEntries(
	cfg *RepairConfig[K],
	invalidEntries []invalidEntry[K],
	result *RepairResult,
) []invalidReport[K] {
	invalidReports := make([]invalidReport[K], 0, len(invalidEntries))

	for _, invalid := range invalidEntries {
		path := s.pathForVersion(invalid.version)
		report := invalidReport[K]{
			key:  invalid.key,
			path: path,
		}

		if !cfg.DryRun {
			err := s.index.Delete(bgCtx, invalid.key)
			if err != nil && !errors.Is(err, cache.ErrNotFound) {
				report.removeErr = err
				s.reportRepairError(cfg, err)
			}

			if err == nil {
				result.RemovedEntries++
			}
		}

		invalidReports = append(invalidReports, report)
	}

	return invalidReports
}

func (s *Storage[K]) collectPendingDeadVersions() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	pendingDead := make(map[string]struct{}, len(s.openedFiles))

	for version, opened := range s.openedFiles {
		if opened == nil || !opened.dead {
			continue
		}

		pendingDead[version] = struct{}{}
	}

	return pendingDead
}

func (s *Storage[K]) collectOrphanPaths(cfg *RepairConfig[K], liveVersions map[string]struct{}) []string {
	pendingDead := s.collectPendingDeadVersions()
	orphanPaths := make([]string, 0)

	_ = filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			s.reportRepairError(cfg, err)

			return nil
		}

		if d.IsDir() || filepath.Ext(d.Name()) != fileExt {
			return nil
		}

		version := d.Name()[:len(d.Name())-len(fileExt)]

		if _, ok := liveVersions[version]; ok {
			return nil
		}

		if _, ok := pendingDead[version]; ok {
			return nil
		}

		orphanPaths = append(orphanPaths, path)

		return nil
	})

	return orphanPaths
}

func (s *Storage[K]) repairOrphanFiles(
	cfg *RepairConfig[K],
	orphanPaths []string,
	result *RepairResult,
) []orphanReport {
	orphanReports := make([]orphanReport, 0, len(orphanPaths))

	for _, path := range orphanPaths {
		report := orphanReport{path: path}

		if !cfg.DryRun {
			err := os.Remove(path)
			if err != nil {
				report.removeErr = err
				s.reportRepairError(cfg, err)
			}

			if err == nil {
				result.RemovedFiles++
			}
		}

		orphanReports = append(orphanReports, report)
	}

	return orphanReports
}

func (s *Storage[K]) reportRepair(
	invalidReports []invalidReport[K],
	orphanReports []orphanReport,
	cfg *RepairConfig[K],
) {
	for _, report := range invalidReports {
		if cfg.OnInvalidEntry == nil {
			continue
		}

		cfg.OnInvalidEntry(report.key, report.path, report.removeErr)
	}

	for _, report := range orphanReports {
		if cfg.OnOrphanFile == nil {
			continue
		}

		cfg.OnOrphanFile(report.path, report.removeErr)
	}
}

func (s *Storage[K]) reportRepairError(cfg *RepairConfig[K], err error, ignored ...error) {
	if err == nil || cfg.OnError == nil {
		return
	}

	for _, ignore := range ignored {
		if errors.Is(err, ignore) {
			return
		}
	}

	cfg.OnError(err)
}

// Close dumps the in-memory index and stops background jobs.
func (s *Storage[K]) Close() error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()

		return nil
	}

	if err := s.flush(); err != nil {
		s.closeMu.Unlock()

		return err
	}

	s.index = nil
	s.closed = true
	s.closeMu.Unlock()

	s.openedWait.Wait()

	return nil
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
		rt.refs--
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
		rt.refs++

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

func (s *Storage[K]) storedBytesOverflow() bool {
	if s.limit == 0 {
		return false
	}

	total := atomic.LoadInt64(&s.bytes)
	if total < 0 {
		total = 0
	}

	if s.limit > math.MaxInt64 {
		return false
	}

	return total > int64(s.limit)
}

func (s *Storage[K]) addStoredBytes(delta int64) {
	if delta == 0 {
		return
	}

	total := atomic.AddInt64(&s.bytes, delta)
	if total >= 0 {
		return
	}

	atomic.StoreInt64(&s.bytes, 0)
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
	if err == nil {
		s.recountStoredBytes()
	}

	return err
}

func (s *Storage[K]) recountStoredBytes() {
	var total int64

	_, _ = s.index.Walk(func(entry cache.EntryBy[K, storedEntry]) error {
		if size := entry.Value().Meta.Size; size > 0 {
			total += size
		}

		return nil
	})

	atomic.StoreInt64(&s.bytes, total)
}

// Flush writes the in-memory index to persistent storage, ensuring data consistency and integrity.
func (s *Storage[K]) Flush() (err error) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed {
		return nil
	}

	return s.flush()
}

func (s *Storage[K]) flush() (err error) {
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

	return joinErr(err, t.relErr)
}

func joinErr(err error, other error) error {
	if err == nil {
		return other
	}

	if other == nil {
		return err
	}

	return fmt.Errorf("%v: %w", err, other)
}
