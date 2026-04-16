//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"time"
)

// ReaderBy reads from cache with a typed key.
type ReaderBy[K comparable, V any] interface {
	// Read returns cached value or error.
	Read(ctx context.Context, key K) (V, error)
}

// WriterBy writes to cache with a typed key.
type WriterBy[K comparable, V any] interface {
	// Write stores value in cache with a given key.
	Write(ctx context.Context, key K, value V) error
}

// ReadWriterBy reads from and writes to cache with a typed key.
type ReadWriterBy[K comparable, V any] interface {
	ReaderBy[K, V]
	WriterBy[K, V]
}

// WalkerBy calls function for every entry in cache and fails on first error returned by that function.
//
// Count of processed entries is returned.
type WalkerBy[K comparable, V any] interface {
	Walk(cb func(entry EntryBy[K, V]) error) (int, error)
}

// EntryBy is cache entry with typed key and value.
type EntryBy[K comparable, V any] interface {
	Key() K
	Value() V
	ExpireAt() time.Time
}
