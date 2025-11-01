//go:build go1.18
// +build go1.18

package cache

import (
	"context"
)

// NoOpOf is a ReadWriterOf stub.
type NoOpOf[V any] struct{}

var (
	_ ReadWriterOf[int] = NoOpOf[int]{}
	_ Deleter           = NoOpOf[int]{}
)

// Write does nothing.
func (NoOpOf[V]) Write(_ context.Context, _ []byte, _ V) error {
	return nil
}

// Read is always missing item.
func (NoOpOf[V]) Read(_ context.Context, _ []byte) (V, error) {
	var v V

	return v, ErrNotFound
}

// Delete is always missing item.
func (NoOpOf[V]) Delete(_ context.Context, _ []byte) error {
	return ErrNotFound
}
