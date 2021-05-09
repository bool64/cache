package cache

import "time"

// SentinelError is an error.
type SentinelError string

const (
	// ErrExpired indicates expired cache entry,
	// may implement ErrWithExpiredItem to enable stale value serving.
	ErrExpired = SentinelError("expired cache item")

	// ErrNotFound indicates missing cache entry.
	ErrNotFound = SentinelError("missing cache item")

	// ErrNothingToInvalidate indicates no caches were added to Invalidator.
	ErrNothingToInvalidate = SentinelError("nothing to invalidate")

	// ErrAlreadyInvalidated indicates recent invalidation.
	ErrAlreadyInvalidated = SentinelError("already invalidated")
)

// Error implements error.
func (e SentinelError) Error() string {
	return string(e)
}

// ErrWithExpiredItem defines an expiration error with entry details.
type ErrWithExpiredItem interface {
	error
	Value() interface{}
	ExpiredAt() time.Time
}
