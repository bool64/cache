package cache

import (
	"context"
)

// NoOp is a ReadWriter stub.
type NoOp struct{}

var _ ReadWriter = NoOp{}

// Write does nothing.
func (NoOp) Write(_ context.Context, _ []byte, _ interface{}) error {
	return nil
}

// Read is always missing item.
func (NoOp) Read(_ context.Context, _ []byte) (interface{}, error) {
	return nil, ErrNotFound
}

// Delete is always missing item.
func (NoOp) Delete(_ context.Context, _ []byte) error {
	return ErrNotFound
}
