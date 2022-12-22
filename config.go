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

	// ItemsCountReportInterval is items count metric report interval, default 1m.
	ItemsCountReportInterval time.Duration

	// Expiration controls.

	// TimeToLive is delay before entry expiration, default 5m.
	// Use UnlimitedTTL value to set up unlimited TTL.
	TimeToLive time.Duration

	// DeleteExpiredAfter is delay before expired entry is deleted from cache, default 24h.
	DeleteExpiredAfter time.Duration

	// DeleteExpiredJobInterval is delay between two consecutive cleanups, default 1h.
	DeleteExpiredJobInterval time.Duration

	// ExpirationJitter is a fraction of TTL to randomize, default 0.1.
	// Use -1 to disable.
	// If enabled, entry TTL will be randomly altered in bounds of ±(ExpirationJitter * TTL / 2).
	ExpirationJitter float64

	// Eviction controls.
	//
	// Eviction is a part of delete expired job, eviction runs at most once per delete expired job and
	// removes a number of entries (up to EvictFraction) based on EvictionStrategy. Eviction only runs if
	// any of soft limits is breached or if user-defined EvictionNeeded returns true.

	// HeapInUseSoftLimit sets heap in use (runtime.MemStats).HeapInuse threshold when eviction will be triggered.
	HeapInUseSoftLimit uint64

	// SysMemSoftLimit sets system memory (runtime.MemStats).Sys threshold when eviction will be triggered.
	SysMemSoftLimit uint64

	// CountSoftLimit sets count threshold when eviction will be triggered.
	// As opposed to memory soft limits, when count limit is exceeded, eviction will remove items to achieve
	// the level of CountSoftLimit*(1-EvictFraction), which may be more items that EvictFraction defines.
	CountSoftLimit uint64

	// EvictionNeeded is a user-defined function to decide whether eviction is necessary.
	// If true is returned, eviction cycle will happen.
	EvictionNeeded func() bool

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
