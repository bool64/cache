//go:build go1.18
// +build go1.18

package cache

// ConfigBy controls typed-key cache instances.
type ConfigBy[K comparable] struct {
	Config

	// ShardFunc customizes shard selection in ShardedMapBy.
	// If nil, a default sharder is used for supported key kinds.
	ShardFunc func(K) uint64
}

// Use is a functional option to apply keyed configuration.
func (c ConfigBy[K]) Use(cfg *ConfigBy[K]) {
	*cfg = c
}
