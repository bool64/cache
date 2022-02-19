//go:build go1.18
// +build go1.18

package cache

import "time"

// ErrWithExpiredItemOf defines an expiration error with entry details.
type ErrWithExpiredItemOf[V any] interface {
	error
	Value() V
	ExpiredAt() time.Time
}
