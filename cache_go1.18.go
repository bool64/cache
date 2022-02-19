//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"time"
)

// ReaderOf reads from cache.
type ReaderOf[V any] interface {
	// Read returns cached value or error.
	Read(ctx context.Context, key []byte) (V, error)
}

// WriterOf writes to cache.
type WriterOf[V any] interface {
	// Write stores value in cache with a given key.
	Write(ctx context.Context, key []byte, value V) error
}

// ReadWriterOf reads from and writes to cache.
type ReadWriterOf[V any] interface {
	ReaderOf[V]
	WriterOf[V]
}

// WalkerOf calls function for every entry in cache and fails on first error returned by that function.
//
// Count of processed entries is returned.
type WalkerOf[V any] interface {
	Walk(func(entry EntryOf[V]) error) (int, error)
}

// EntryOf is cache entry with key and value.
type EntryOf[V any] interface {
	Key() []byte
	Value() V
	ExpireAt() time.Time
}
