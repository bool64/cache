//go:build go1.18
// +build go1.18

package bench

import "github.com/bool64/cache"

func init() {
	Failovers = append(Failovers,
		FailoverOf{F: func() cache.ReadWriterOf[SmallCachedValue] {
			return cache.NewShardedMapOf[SmallCachedValue]()
		}},
	)

	ReadWriters = append(ReadWriters,
		ReadWriterOfRunner{f: func() cache.ReadWriterOf[SmallCachedValue] {
			return cache.NewShardedMapOf[SmallCachedValue]()
		}},
	)

	Baseline = append(Baseline,
		ShardedMapOfBaseline{},
	)
}
