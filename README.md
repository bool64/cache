# High performance resilient in-memory cache for Go

This library defines cache interfaces and provides in-memory implementations.

[![Build Status](https://github.com/bool64/cache/workflows/test-unit/badge.svg)](https://github.com/bool64/cache/actions?query=branch%3Amaster+workflow%3Atest-unit)
[![Coverage Status](https://codecov.io/gh/bool64/cache/branch/master/graph/badge.svg)](https://codecov.io/gh/bool64/cache)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/bool64/cache)
[![time tracker](https://wakatime.com/badge/github/bool64/cache.svg)](https://wakatime.com/badge/github/bool64/cache)
![Code lines](https://sloc.xyz/github/bool64/cache/?category=code)
![Comments](https://sloc.xyz/github/bool64/cache/?category=comments)

## Why?

There are a few libraries that provide in-memory cache already, why another one?

This library addresses additional practical issues that are not usually covered by key-value storage concerns. It helps
to improve performance and resiliency by gentle handling of cache misses and allows for comprehensive observability with 
fine control of caching behavior.

Please check this [blog post](https://dev.to/vearutop/implementing-robust-in-memory-cache-with-go-196e) for more details.

## Failover Cache

[`Failover`](https://pkg.go.dev/github.com/bool64/cache#Failover) is a cache frontend to manage cache updates in a
non-conflicting and performant way.

An instance can be created with [`NewFailover`](https://pkg.go.dev/github.com/bool64/cache#NewFailover) and functional
options.

Main API is a `Get` function that takes a key and a builder function. If value is available in cache, it is served from
cache and builder function is not invoked. If value is not available in cache, builder function is invoked and the
result is stored in cache.

```go
// Get value from cache or the function.
v, err := f.Get(ctx, []byte("my-key"), func(ctx context.Context) (interface{}, error) {
    // Build value or return error on failure.

    return "<value>", nil
})
```

Or, starting with go1.18 you can use generic API.

```go
f := cache.NewFailoverOf[Dog]()

// Get value from cache or the function.
v, err := f.Get(ctx, []byte("my-key"), func(ctx context.Context) (Dog, error) {
    // Build value or return error on failure.

    return Dog{Name: "Snoopy"}, nil
})
```

Additionally, there are few other aspects of behavior to optimize performance.

* Builder function is locked per key, so if the key needs a fresh value the builder function is only called once. All
  the other `Get` calls for the same key are blocked until the value is available. This helps to avoid
  [cache stampede problem](https://en.wikipedia.org/wiki/Cache_stampede) when popular value is missing or
  expired.
* If expired (stale) value is available, the value is refreshed with a short TTL (configured as `UpdateTTL`) before the
  builder function is invoked. This immediately unblocks readers with a stale value and improves tail latency.
* If the value has expired longer than `MaxStaleness` ago, stale value is not served and readers are blocked till the
  builder function return.
* By default, if stale value is served, it is served to all readers, including the first reader who triggered builder
  function. Builder function runs in background so that reader latency is not affected. This behavior can be changed
  with `SyncUpdate` option, so that first reader who invokes builder function is blocked till result is ready instead of
  having stale value immediately.
* If builder function fails, the error value is also cached and all consecutive calls for the key, would fail
  immediately with same error for next 20 seconds (can be configured with `FailedUpdateTTL`). This helps to avoid
  abusing building function when there is a persistent problem. For example, if you have 100 hits per second for a key
  that is updated from database and database is temporary down, errors caching prevents unexpected excessive load that
  usually hides behind value cache.
* If builder function fails and stale value is available, stale value is served regardless of `MaxStaleness`. This
  allows to reduce impact of temporary outages in builder function. This behavior can be disabled with `FailHard`
  option, so that error is served instead of overly stale value.

`Failover` cache uses [`ReadWriter`](https://pkg.go.dev/github.com/bool64/cache#ReadWriter) backend as a storage. By
default [`ShardedMap`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap) is created using `BackendConfig`.

It is recommended that separate caches are used for different entities, this helps observability on the sizes and
activity for particular entities. Cache `Name` can be configured to reflect the purpose. Additionally, `Logger`
and `Stats` tracker can be provided to collect operating information.

If `ObserveMutability` is enabled, `Failover` will also emit stats of how often the rebuilt value was different from the
previous. This may help to understand data volatility and come up with a better TTL value. The check is done
with [`reflect.DeepEqual`](https://pkg.go.dev/reflect#DeepEqual) and may affect performance.

## Sharded Map

[`ShardedMap`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap)
implements [`ReadWriter`](https://pkg.go.dev/github.com/bool64/cache#ReadWriter) and few other behaviours with in-memory
storage sharded by key. It offers good performance for concurrent usage. Values can expire.

An instance can be created with [`NewShardedMap`](https://pkg.go.dev/github.com/bool64/cache#NewShardedMap) and
functional options.

Generic API is also available with [`NewShardedMapOf`](https://pkg.go.dev/github.com/bool64/cache#NewShardedMapOf).

It is recommended that separate caches are used for different entities, this helps observability on the sizes and
activity for particular entities. Cache `Name` can be configured to reflect the purpose. Additionally, `Logger`
and `Stats` tracker can be provided to collect operating information.

Expiration is configurable with `TimeToLive` and defaults to 5 minutes. It can be changed to a particular key via
context by [`cache.WithTTL`](https://pkg.go.dev/github.com/bool64/cache#WithTTL).

Actual TTL applied to a particular key is randomly altered in ±5% boundaries (configurable with `ExpirationJitter`),
this helps against synchronous cache expiration (and excessive load to refresh many values at the same time) in case
when many cache entries were created within a small timeframe (for example early after application startup). Expiration
jitter diffuses such synchronization for smoother load distribution.

Expired items are not deleted immediately to reduce the churn rate and to provide stale data for `Failover` cache.

All items are checked in background once an hour (configurable with `DeleteExpiredJobInterval`) and items that have
expired more than 24h ago (configurable with `DeleteExpiredAfter`) are removed.

Additionally, there are `HeapInUseSoftLimit` and `CountSoftLimit` to trigger eviction of 10% (configurable
with `EvictFraction`) entries if count of items or application heap in use exceeds the limit. Limit check and
optional eviction are triggered right after expired items check (in the same background job).

`EvictionStrategy` defines which entries would be evicted, by default `EvictMostExpired` is used.
It selects entries with the longest expiration overdue or those that are soonest to expire.

Alternatively `EvictLeastRecentlyUsed` (LRU) and `EvictLeastFrequentlyUsed` (LFU) can be used at cost 
of minor performance impact (for updating counters on each cache serve).

Keep in mind that eviction happens in response to soft limits that are checked periodically, so
dataset may stay above eviction threshold, especially if `EvictFraction` combined with `DeleteExpiredJobInterval` 
are lower than speed of growth.

### Batch Operations

[`ShardedMap`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap)
has [`ExpireAll`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.ExpireAll) function to mark all entries as
expired, so that they are updated on next read and are available as stale values in meantime, this function does not
affect memory usage.

In contrast, [`DeleteAll`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.DeleteAll) removes all entries and
frees the memory, stale values are not available after this operation.

Deleting or expiring all items in multiple caches can be done with help
of [`cache.Invalidator`](https://pkg.go.dev/github.com/bool64/cache#Invalidator). Deletion/expiration function can be
appended to `Invalidator.Callbacks` and it will be triggered
on [`Invalidator.Invalidate`](https://pkg.go.dev/github.com/bool64/cache#Invalidator.Invalidate). This may be useful as
a debugging/firefighting tool.

Deleting of multiple related (labeled) items can be done with 
[`InvalidationIndex`](ttps://pkg.go.dev/github.com/bool64/cache#InvalidationIndex).

[`Len`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.Len) returns currently available number of entries (
including expired).

[`Walk`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.Walk) iterates all entries and invokes a callback for
each entry, iteration stops if callback fails.

Cached entries can be dumped as a binary stream
with [`Dump`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.Dump) and restored from a binary stream
with [`Restore`](https://pkg.go.dev/github.com/bool64/cache#ShardedMap.Restore), this may enable cache transfer between
the instances of an application to avoid cold state after startup. Binary serialization is done
with [`encoding/gob`](https://pkg.go.dev/encoding/gob), cached types that are to be dumped/restored have to be
registered with [`cache.GobRegister`](https://pkg.go.dev/github.com/bool64/cache#GobRegister).

Dumping and walking cache are non-blocking operations and are safe to use together with regular reads/writes,
performance impact is expected to be negligible.

[`HTTPTransfer`](https://pkg.go.dev/github.com/bool64/cache#HTTPTransfer) is a helper to transfer caches over HTTP.
Having multiple cache instances registered, it
provides [`Export`](https://pkg.go.dev/github.com/bool64/cache#HTTPTransfer.Export) HTTP handler that can be plugged
into the HTTP server and serve data for an [`Import`](https://pkg.go.dev/github.com/bool64/cache#HTTPTransfer.Import)
function of another application instance.

[`HTTPTransfer.Import`](https://pkg.go.dev/github.com/bool64/cache#HTTPTransfer.Import) fails if cached types differ
from the exporting application instance, for example because of different versions of applications. The check is based
on [`cache.GobTypesHash`](https://pkg.go.dev/github.com/bool64/cache#GobTypesHash)
that is calculated from cached structures
during [`cache.GobRegister`](https://pkg.go.dev/github.com/bool64/cache#GobRegister).

## Sync Map

[`SyncMap`](https://pkg.go.dev/github.com/bool64/cache#SyncMap)
implements [`ReadWriter`](https://pkg.go.dev/github.com/bool64/cache#ReadWriter) and few other behaviours with in-memory
storage backed by standard [`sync.Map`](https://pkg.go.dev/sync#Map). It implements same behaviors
as [`ShardedMap`](#sharded-map) and can be a replacement. There is slight performance difference in latency and
usually `ShardedMap` tends to consume less memory.

## Context

Context is propagated from parent goroutine to `Failover` and further to backend `ReadWriter` and builder function. In
addition to usual responsibilities (cancellation, tracing, etc...), context can carry cache options.

* [`cache.WithTTL`](https://pkg.go.dev/github.com/bool64/cache#WithTTL)
  and [`cache.TTL`](https://pkg.go.dev/github.com/bool64/cache#TTL) to set and get time to live for a particular
  operation.
* [`cache.WithSkipRead`](https://pkg.go.dev/github.com/bool64/cache#WithSkipRead)
  and [`cache.SkipRead`](https://pkg.go.dev/github.com/bool64/cache#SkipRead) to set and get skip reading flag, if the
  flag is set `Read` function should return `ErrNotFound`, therefore bypassing cache. At the same time `Write` operation
  is not affected by this flag, so `SkipRead` can be used to force cache refresh.

A handy use case for [`cache.WithSkipRead`](https://pkg.go.dev/github.com/bool64/cache#WithSkipRead) could be to
implement a debug mode for request processing with no cache. Such debug mode can be implemented with HTTP (or other
transport) middleware that instruments context under certain conditions, for example if a special header is found in
request.

### Detached Context

When builder function is invoked in background, the context is detached into a new one, derived context is not
cancelled/failed/closed if parent context is.

For example, original context was created for an incoming HTTP request and was closed once response was written,
meanwhile the cache update that was triggered in this context is still being processed in background. If original
context was used, background processing would have been cancelled once parent HTTP request is fulfilled, leading to
update failure.

Detached context makes background job continue even after the original context was legitimately closed.  

## Performance

`Failover` cache adds some overhead, but overall performance is still good (especially for IO-bound applications).

Please check detailed [benchmarks](./_benchmark/README.md).