//go:build go1.18
// +build go1.18

package bench

import "github.com/bool64/cache"

func init() {
	Failovers = append(Failovers,
		FailoverOf{F: func() cache.ReadWriterOf[SmallCachedValue] {
			return cache.NewShardedMapOf[SmallCachedValue](func(cfg *cache.ConfigOf[SmallCachedValue]) {
				cfg.TimeToLive = cache.UnlimitedTTL
			})
		}},
	)

	ReadWriters = append(ReadWriters,
		ReadWriterOfRunner{F: func() cache.ReadWriterOf[SmallCachedValue] {
			return cache.NewShardedMapOf[SmallCachedValue]()
		}},
	)

	Baseline = append(Baseline,
		SyncMapByBaseline{},
		ShardedMapByBaseline{},
		ShardedMapOfBaseline{},
	)
}
