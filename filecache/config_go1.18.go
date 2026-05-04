//go:build go1.18
// +build go1.18

package filecache

import (
	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
)

// Config controls file-backed storage layout and index behavior.
type Config[K comparable] struct {
	// IndexPolicy configures the in-memory index backend.
	IndexPolicy cache.Policy

	// RetentionPolicy configures blob-storage retention and eviction behavior.
	blob.RetentionPolicy

	// IndexShardFunc customizes shard selection in the typed in-memory index.
	IndexShardFunc func(K) uint64

	// SplitPath converts a version into nested path segments under data dir.
	// If nil, two 2-character prefix directories are used.
	SplitPath func(version string) []string
}

// Use is a functional option to apply storage configuration.
func (c Config[K]) Use(cfg *Config[K]) {
	*cfg = c
}

// PrefixSplit creates a version path splitter that uses consecutive prefix lengths
// as nested directory names. For example PrefixSplit(1) uses the first character,
// and PrefixSplit(2, 2) matches the default layout.
func PrefixSplit(lengths ...int) func(version string) []string {
	cp := append([]int(nil), lengths...)

	return func(version string) []string {
		segments := make([]string, 0, len(cp))
		pos := 0

		for _, n := range cp {
			if n <= 0 || pos >= len(version) {
				break
			}

			end := pos + n
			if end > len(version) {
				end = len(version)
			}

			segments = append(segments, version[pos:end])
			pos = end
		}

		return segments
	}
}
