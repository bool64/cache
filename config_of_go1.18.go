//go:build go1.18
// +build go1.18

package cache

// ConfigOf controls []byte-keyed typed-value cache instances.
type ConfigOf[V any] struct {
	Policy

	// OnDelete is called when an entry is removed from cache by Delete, DeleteAll, expiration cleanup, or eviction.
	OnDelete func(key []byte, value V)
}

// Use is a functional option to apply typed-value configuration.
func (c ConfigOf[V]) Use(cfg *ConfigOf[V]) {
	*cfg = c
}

// WithPolicyOf applies shared policy to []byte-keyed typed-value cache configuration.
func WithPolicyOf[V any](policy Policy) func(*ConfigOf[V]) {
	return func(cfg *ConfigOf[V]) {
		cfg.Policy = policy
	}
}
