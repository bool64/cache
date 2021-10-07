//go:build go1.18
// +build go1.18

package cache

import "context"

// ReaderOf reads from cache.
type ReaderOf[value any] interface {
	// Read returns cached value or error.
	Read(ctx context.Context, key []byte) (value, error)
}

// WriterOf writes to cache.
type WriterOf[value any] interface {
	// Write stores value in cache with a given key.
	Write(ctx context.Context, key []byte, value value) error
}

// ReadWriterOf reads from and writes to cache.
type ReadWriterOf[value any] interface {
	ReaderOf[value]
	WriterOf[value]
}
