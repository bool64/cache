//go:build go1.18
// +build go1.18

package cache

// ConfigBy controls typed-key cache instances.
type ConfigBy[K comparable, V any] struct {
	Policy

	// ShardFunc customizes shard selection in ShardedMapBy.
	// If nil, a default sharder is used for supported key kinds.
	ShardFunc func(K) uint64

	// OnDeleteBy is called when an entry is removed from a typed-key cache by Delete,
	// DeleteAll, expiration cleanup, or eviction.
	OnDeleteBy func(key K, value V)
}

// Use is a functional option to apply keyed configuration.
func (c ConfigBy[K, V]) Use(cfg *ConfigBy[K, V]) {
	*cfg = c
}

// WithPolicyBy applies shared policy to typed-key cache configuration.
func WithPolicyBy[K comparable, V any](policy Policy) func(*ConfigBy[K, V]) {
	return func(cfg *ConfigBy[K, V]) {
		cfg.Policy = policy
	}
}
