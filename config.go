package cache

import "time"

// Config controls cache instance.
type Config struct {
	// Logger is an instance of contextualized logger, can be nil.
	Logger Logger

	// Stats is a metrics collector, can be nil.
	Stats StatsTracker

	// Name is cache instance name, used in stats and logging.
	Name string

	// TimeToLive is delay before entry expiration, default 5m.
	// Use UnlimitedTTL value to set up unlimited TTL.
	TimeToLive time.Duration

	// DeleteExpiredAfter is delay before expired entry is deleted from cache, default 24h.
	DeleteExpiredAfter time.Duration

	// DeleteExpiredJobInterval is delay between two consecutive cleanups, default 1h.
	DeleteExpiredJobInterval time.Duration

	// ItemsCountReportInterval is items count metric report interval, default 1m.
	ItemsCountReportInterval time.Duration

	// ExpirationJitter is a fraction of TTL to randomize, default 0.1.
	// Use -1 to disable.
	// If enabled, entry TTL will be randomly altered in bounds of ±(ExpirationJitter * TTL / 2).
	ExpirationJitter float64

	// HeapInUseSoftLimit sets heap in use threshold when eviction of most expired items will be triggered.
	//
	// Eviction is a part of delete expired job, eviction runs at most once per delete expired job and
	// removes most expired entries up to EvictFraction.
	HeapInUseSoftLimit uint64

	// CountSoftLimit sets count threshold when eviction of most expired items will be triggered.
	//
	// Eviction is a part of delete expired job, eviction runs at most once per delete expired job and
	// removes most expired entries up to EvictFraction.
	CountSoftLimit uint64

	// EvictFraction is a fraction (0, 1] of total count of items to be evicted when resource is overused,
	// default 0.1 (10% of items).
	EvictFraction float64

	// EvictionStrategy is EvictMostExpired by default.
	EvictionStrategy EvictionStrategy
}

// EvictionStrategy defines eviction behavior when soft limit is met during cleanup job.
type EvictionStrategy uint8

const (
	// EvictMostExpired removes entries with the oldest expiration time.
	// Both expired and non-expired entries may be affected.
	// Default eviction strategy, most performant as it does not maintain counters on each serve.
	EvictMostExpired EvictionStrategy = iota

	// EvictLeastRecentlyUsed removes entries that were not served recently.
	// It has a minor performance impact due to update of timestamp on every serve.
	EvictLeastRecentlyUsed

	// EvictLeastFrequentlyUsed removes entries that were in low demand.
	// It has a minor performance impact due to update of timestamp on every serve.
	EvictLeastFrequentlyUsed
)

// Use is a functional option to apply configuration.
func (c Config) Use(cfg *Config) {
	*cfg = c
}
