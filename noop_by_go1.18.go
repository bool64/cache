//go:build go1.18
// +build go1.18

package cache

import "context"

// NoOpBy is a ReadWriterBy stub.
type NoOpBy[K comparable, V any] struct{}

var _ ReadWriterBy[string, int] = NoOpBy[string, int]{}

// Write does nothing.
func (NoOpBy[K, V]) Write(_ context.Context, _ K, _ V) error {
	return nil
}

// Read is always missing item.
func (NoOpBy[K, V]) Read(_ context.Context, _ K) (V, error) {
	var v V

	return v, ErrNotFound
}

// Delete is always missing item.
func (NoOpBy[K, V]) Delete(_ context.Context, _ K) error {
	return ErrNotFound
}
