# Blob Cache Design

## Status

This document captures the current design direction for a blob-oriented cache ecosystem.

The goal is to preserve context across sessions and evolve this document as the design becomes more concrete.

## Motivation

The main use case is serving photos or other files in an HTTP service.

Typical flow:

1. A request needs a file identified by cache key.
2. On cache miss or stale entry, the cache downloads the file from an origin server.
3. The cache stores file content in local persistent storage.
4. The cache returns an object usable by HTTP-serving code without loading the whole file into memory.

Important requirements:

- persistent local-file-backed cache
- in-memory lightweight index of entries
- janitor behavior similar to existing in-memory cache
- compatibility with failover/stale serving behavior
- support for preserving relevant HTTP response metadata
- no requirement to materialize full file content into `[]byte`

## Package Direction

Current preference is to split responsibilities like this:

- `cache`
  - generic cache primitives such as `FailoverOf[T]`, `ReadWriterOf[T]`, `ShardedMapOf[T]`
- `blob`
  - generic blob abstractions and helper constructors
- `filecache`
  - local filesystem-backed implementation of `cache.ReadWriterOf[blob.Entry]`
- `sqlcache`
  - SQL/DB-backed implementation of `cache.ReadWriterOf[blob.Entry]`

Rationale:

- keeps root `cache` package generic
- gives blob abstractions a backend-neutral home
- gives helper constructors such as HTTP-response adapters a natural home
- avoids making blob abstractions look local-file-specific when they are also useful for DB/S3/custom storage

## Core Object Model

Current preferred exported names:

- `blob.Entry`
- `blob.Meta`

### `Meta`

`Meta` carries content metadata that should travel with the object.

Likely fields:

```go
type Meta struct {
	Name    string
	Size    int64
	ModTime time.Time
	Extra   any
}
```

Notes:

- `Name`, `Size`, and `ModTime` are considered common fields for arbitrary blob-like content.
- Domain-specific metadata should live in `Extra`.
- `Extra any` is preferred over `map[string]any` because it allows callers to pass typed payloads.
- For HTTP-oriented usage, `Extra` can hold a typed struct with response status/headers.
- Opaque `Extra` data is expected to be round-tripped by storage/index implementations.

Example of HTTP-oriented extension payload:

```go
type HTTPExtra struct {
	StatusCode int
	Header     http.Header
}
```

### `Entry`

Current preferred shape:

```go
type Entry interface {
	Meta() Meta
	Open() (io.ReadCloser, error)
}
```

Rationale:

- this is the stable value to keep inside `FailoverOf` and cache backends
- it is compact and durable
- it can live inside `ShardedMapBy[K, blob.Entry]` or any `ReadWriterOf[blob.Entry]`
- it separates cached value identity from ephemeral opened handles
- it avoids forcing all blob backends to expose the same advanced reader capabilities

Important distinction:

- `Entry` is the cached descriptor/reference
- the result of `Open()` is the opened stream handle

This distinction is now the key mechanism for reusing existing cache primitives safely.

## Two Kinds of Entry Implementations

The same `blob.Entry` abstraction is expected to have two distinct implementation classes.

### 1. Transient source entry

Example source:

- `blob.FromHTTPResponse(resp)`

This entry is produced by the builder callback and wraps origin content.

Expected behavior:

- `Meta()` works
- `Open()` returns a readable and closable source
- advanced capabilities such as `Seek` and `ReadAt` may be absent

Typical implementations:

- response-backed source entry
- reader-backed source entry

### 2. Stored entry

This entry is returned from cache storage and knows how to open a durable blob.

Expected behavior:

- `Meta()` works
- `Open()` returns a readable and closable handle
- richer capabilities may be present depending on storage/backend

Typical implementations:

- local-file-backed entry opening `*os.File`
- DB-backed entry opening a DB/blob reader
- object-storage-backed entry opening a network or buffered reader

## Capability Diversity Across Storages

Different blob storages may offer different capabilities.

Examples:

- local file: `Reader`, `ReaderAt`, `Seeker`, `Closer`
- HTTP response body: `Reader`, `Closer`
- database blob: often `Reader`, sometimes random access depending on driver
- S3 object: stream by default, random access may require range requests or buffering

Important conclusion:

- absence of `Seek` is not necessarily a deal-breaker for the overall design

Reasoning:

- some user code can work around missing seekability in userland
- for example by reading all content and wrapping it with `bytes.NewReader`
- this has a resource cost, but it preserves compatibility when random access is needed

Current stance:

- `blob.Entry.Open()` returns `io.ReadCloser`
- concrete returned readers may also implement `io.Seeker`, `io.ReaderAt`, or both
- consumers that need those capabilities can use type assertions
- storage implementations are free to expose richer capabilities when practical
- implementations should avoid wrappers like `io.NopCloser` when they would hide useful reader interfaces
- capability-preserving wrappers are preferred when a close method must be added to a richer reader

This keeps the API simple while avoiding the assumption that all backends are naturally file-like.

## Construction Helpers

Current preferred helper names:

- `blob.FromHTTPResponse(resp)`
- `blob.FromReader(r, meta)`

Likely additional helpers:

- `blob.FromReadCloser(rc, meta)`
- maybe `blob.FromFile(f, meta)`

Meaning of these constructors:

- they produce ingestible transient `blob.Entry` values
- cache manager/storage reads bytes from `Open()` and persists the result into durable storage
- they are not necessarily the final entry form returned to users

## Failover Integration

The strongest current direction is to maximize reuse of existing `cache.FailoverOf` and `ReadWriterOf`.

The key observation is:

- `FailoverOf` expects a stable cached value
- opened file/blob handles are not stable cached values
- `blob.Entry` is the right stable cached value

Therefore the file/blob cache layer should likely be built around:

- `FailoverOf[blob.Entry]`
- `ReadWriterOf[blob.Entry]`

instead of:

- `FailoverOf[io.ReadCloser]`

This avoids duplicating failover logic and lets the existing cache backends participate.

Desired usage shape:

```go
	entry, err := fc.Get(ctx, key, func(ctx context.Context) (blob.Entry, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	return blob.FromHTTPResponse(resp), nil
})
```

Important intended property:

- the cache instance should conceptually match a `FailoverOf`-style interface

Meaning:

- one build callback per key on miss/stale refresh
- stale serving behavior similar to current failover cache
- result type inside failover/cache storage is `blob.Entry`
- the caller then opens an `Object` from the entry

Potential thin wrapper:

- a higher-level helper could still expose `Get(...)(io.ReadCloser, Meta, error)` or a similar convenience API by internally calling `entry.Open()`
- but the reusable primitive should be `blob.Entry`

## Why `FailoverOf[io.ReadCloser]` Is A Bad Fit

This point was clarified during design discussion.

If `io.ReadCloser` were used as the `V` in `FailoverOf[V]`, current `FailoverOf` behavior would become problematic because:

- `backend.Read` returns an opened, stateful handle
- stale refresh reuses that returned value and calls `backend.Write(..., val)`
- `Write` would need to consume `Read` from the passed object to persist it
- consuming the stream would mutate or exhaust the same object later returned to the caller

This is acceptable for immutable value objects, but not for open stream handles.

That is the main reason `blob.Entry` is now preferred as the cached value.

## Storage Write Semantics

When `ReadWriterOf[blob.Entry].Write(ctx, key, entry)` is invoked for a storage-backed implementation, the expected behavior is:

1. call `entry.Open()`
2. get an `io.ReadCloser`
3. stream bytes from the object's reader into durable storage
4. persist associated metadata / locator
5. close the transient source object
6. store and later return a compact stored `blob.Entry`

For local filesystem storage, this likely means:

1. open source object
2. write to a temp file such as `./storage/<derived>.tmp`
3. atomically rename to `./storage/<derived>.dat`
4. store a `blob.Entry` that knows how to reopen that file

This general pattern also works for other backends such as DB or object storage.

## Storage Implementations

### `filecache.Storage`

Current direction:

- `filecache.Storage` implements `cache.ReadWriterOf[blob.Entry]`
- `filecache.NewStorage(path string)` creates an internal hot index, likely `cache.ShardedMapOf[blob.Entry]`
- if an index file exists on startup, it is restored into that internal map
- `Read` serves from the in-memory descriptor index
- `Write` materializes incoming entry data into local files and updates the index
- `Close()` is required and should dump the in-memory index back to disk

`Close()` is necessary because finalizers are not reliable during application shutdown.

This dump/restore approach is considered suitable for local filesystem storage.

### `sqlcache.BlobStorage`

Current direction:

- `sqlcache.BlobStorage` implements `cache.ReadWriterOf[blob.Entry]`
- `Read` performs a `SELECT`
- `Write` performs an `INSERT/UPDATE` or equivalent
- the DB itself serves as durable storage for index metadata and possibly blob content
- no dump/restore step is required

Important note:

- a SQL-backed implementation may be shared by multiple application instances
- local per-process `FailoverOf` locking does not become distributed locking automatically
- shared storage may improve hit ratio across instances but does not by itself eliminate cross-instance duplicate rebuilds
- cluster-wide stampede prevention is explicitly out of scope for the initial design

## `filecache` Internal State Model

`filecache` should keep two kinds of state.

### 1. Persistent entry state

This is the value stored in the internal `cache.ShardedMapOf[blob.Entry]` index and dumped/restored on disk.

Likely shape:

```go
type entry struct {
	meta    blob.Meta
	version string
	path    string
}
```

Purpose:

- enough information to reopen the current blob after restart
- compact enough to stay in the in-memory descriptor index

### 2. Ephemeral runtime state

This state is process-local and must not be persisted.

Likely shape:

```go
type runtimeFile struct {
	path string
	refs int64
	dead bool
}
```

It should live in a plain mutex-protected map inside `filecache.Storage`.

Rationale:

- the operations guarded by this map are slow file operations anyway
- open/close/version-swap frequency does not justify a more complex structure
- this state is only needed for runtime refcounting and garbage collection

Likely owner state:

```go
type Storage struct {
	index *cache.ShardedMapOf[blob.Entry]

	mu      sync.Mutex
	runtime map[string]*runtimeFile
}
```

Where the map key is likely `version` or a version-derived path.

## Concurrency Model

If multiple callers request the same key concurrently:

1. At most one build should run for that key.
2. Persisted content should be stored once.
3. Each caller should receive its own independently opened stored reader.

Important invariant:

- callers must not share the same `*os.File` handle

Reason:

- reader state is mutable
- `Seek` state may be mutable when supported
- callers may read concurrently
- each request needs an isolated handle

So even if one cached entry points to one persisted blob/file, each `blob.Entry.Open()` should create a fresh opened reader.

## Versioning And Garbage Collection

Current preferred strategy:

- every successful `Write` creates a new immutable versioned file
- the index atomically switches to the new current version
- the previous version becomes garbage immediately after index swap
- each `Open()` increments a runtime refcount for that version
- each returned reader wrapper decrements that refcount on `Close()`
- if a version is marked garbage and its refcount drops to zero, it should be deleted immediately

This gives prompt cleanup without relying on platform-specific behavior of deleting open files.

Janitor remains useful as fallback/reconciliation:

- clean orphaned files left after crashes
- clean leaked temp files
- reconcile disk contents with current index state

`Close()` should also run a best-effort final sweep.

## Close Semantics

Current preference:

- users must call `Close()` on readers returned by `Open()`

Reasoning:

- returned stored objects are expected to hold OS resources
- this matches standard `os.File` usage
- this enables direct use with `http.ServeContent`

Typical handler shape:

```go
entry, err := fc.Get(...)
if err != nil {
	return err
}

rc, err := entry.Open()
if err != nil {
	return err
}
defer rc.Close()

if rs, ok := rc.(io.ReadSeeker); ok {
	http.ServeContent(w, r, entry.Meta().Name, entry.Meta().ModTime, rs)
}
```

Alternative considered but not chosen yet:

- return a metadata object with `Open()`

That may still be reconsidered later if resource-lifetime ergonomics become a concern.

## Persistent Storage Model

Initial focus remains local filesystem storage, but index durability is now considered backend-specific rather than universal.

For local filesystem storage:

- cache content is stored as files on disk
- an in-memory metadata index may be loaded on creation
- that index may be dumped on close
- janitor removes expired/evicted data files together with index entries
- configurable nested directory levels based on leading characters of generated file names remain relevant
- immutable versioned file names are preferred over in-place overwrites

For DB-backed storage:

- the DB can serve as durable blob store and durable index
- dump/restore of an in-memory index is not necessary
- an in-memory cache may still exist as an optimization, but not as the source of truth

## HTTP Metadata Preservation

The origin response may contain useful metadata that should survive caching.

Examples:

- `Content-Type`
- `Content-Length`
- `ETag`
- `Last-Modified`
- `Cache-Control`
- `Content-Disposition`

Current leaning:

- place HTTP-specific values into a typed `Meta.Extra` payload
- store and reload that payload with the entry
- let serving code interpret/filter them when replaying them
- for `filecache`, any `Meta.Extra` payload used with dump/restore must be gob-compatible
- this gob requirement is a `filecache` implementation detail, not a `blob` package contract

Example:

```go
type HTTPExtra struct {
	StatusCode int
	Header     http.Header
}
```

## Why Not Use Existing Value-Oriented Cache API

Using existing `Read(ctx, key) -> interface{}` style APIs is considered a poor fit for this use case.

Reasoning:

- file serving should not require loading whole content into memory
- file objects are stream-oriented and resource-owning
- persistent file-backed semantics differ from current in-memory value cache semantics

Therefore current direction is:

- separate package
- file/blob-oriented API
- maximize reuse of existing root cache interfaces by storing `blob.Entry` values inside them
- avoid duplicating failover logic where `FailoverOf[blob.Entry]` is sufficient

## Generalization Beyond Local Files

There is interest in making the design extensible to other storage backends such as S3 or custom stores.

This is now a more central design goal.

The current direction is:

- treat the cached value as backend-neutral `blob.Entry`
- let different storage implementations define concrete `blob.Entry` forms
- integrate those entries with existing cache backends such as `ShardedMapBy[K, blob.Entry]`

Conceptually:

1. builder returns a transient entry
2. storage-specific `Write` persists it
3. cached backend stores a compact `blob.Entry`
4. callers open fresh readers from that entry

This would allow:

- local filesystem backend
- S3-backed backend
- DB-backed blob backend
- custom user-defined backends

Open concern:

- some backends do not naturally support `Seek` and `ReadAt`
- the core API does not require them
- some backends may expose them via concrete return types from `Open()`
- some backends may emulate them with extra buffering or range requests
- userland may choose to wrap content with `bytes.NewReader` when random access is required
- this flexibility is acceptable even at some performance/resource cost

## Relation To Existing Sharded Map

Earlier implementation attempt duplicated a dedicated sharded metadata map for file cache.

Better direction identified during discussion:

- keep the in-memory part lightweight and close to existing `ShardedMap`
- reuse `ShardedMapBy[K, blob.Entry]` or other existing `ReadWriterOf[blob.Entry]` implementations where possible
- only add hooks/extensions if storage coordination cannot be expressed cleanly in `Read`/`Write`

Rationale:

- reuse proven in-memory indexing behavior
- reuse janitor/eviction behavior where possible
- avoid maintaining parallel sharded-index logic
- let `blob.Entry` be the compact index resident in memory/cache backends

This area remains open for implementation design.

## Open Questions

At this point, architectural questions are mostly settled.

The remaining choices are implementation details that can be decided during coding:

1. exact internal entry/runtime struct layouts in `filecache`
2. exact version id and path naming scheme
3. exact startup reconciliation logic for orphaned files and temp files
4. exact `Close()` sequencing for dump, janitor stop, and final sweep
5. exact helper set to add later for ergonomic `io.ReadSeeker` opening/fallbacks

## Current Preferred Direction Summary

At this point the preferred design is:

- introduce a dedicated `blob` package
- use `blob.Entry` and `blob.Meta`
- support helpers such as `blob.FromHTTPResponse(resp)` and `blob.FromReader(r, meta)`
- make `Meta` contain common fields plus `Extra any`
- make the stable cached value be `blob.Entry`
- maximize reuse of `cache.FailoverOf[blob.Entry]` and `ReadWriterOf[blob.Entry]`
- treat origin-backed response objects as transient ingest sources
- let storage-backed `Write` materialize transient sources into durable blobs
- make `blob.Entry.Open()` return `io.ReadCloser`
- let concrete opened readers optionally expose richer interfaces such as `io.Seeker` and `io.ReaderAt`
- require users to `Close()` returned readers
- implement storage backends such as `filecache.Storage` and `sqlcache.BlobStorage`
- reuse existing cache backends such as `ShardedMapBy[K, blob.Entry]` wherever possible
- support various blob storages including local files, DB blobs, S3-like stores, and custom user implementations
- for `filecache`, use immutable versioned files plus runtime refcount-based garbage collection
- keep runtime refcount/dead-file state in a plain mutex-protected map inside `filecache.Storage`
- make `filecache.Storage.Close()` explicit and required
- keep checksum/hash standardization out of the public API for now and, if needed, represent such details inside `Meta.Extra` or backend-private state
- for now, make storage `Write` always rewrite/materialize incoming content rather than trying to short-circuit reuse
