package cache

const (
	// MetricMiss is a name of a metric to count cache miss events.
	MetricMiss = "cache_miss"
	// MetricExpired is a name of a metric to count expired cache read events.
	MetricExpired = "cache_expired"
	// MetricHit is a name of a metric to count valid cache read events.
	MetricHit = "cache_hit"
	// MetricWrite is a name of a metric to count cache write events.
	MetricWrite = "cache_write"
	// MetricItems is a name of a gauge to count number of items in cache.
	MetricItems = "cache_items"

	// MetricRefreshed is a name of a metric to count stale refresh events.
	MetricRefreshed = "cache_refreshed"
	// MetricBuild is a name of a metric to count value building events.
	MetricBuild = "cache_build"
	// MetricFailed is a name of a metric to count number of failed value builds.
	MetricFailed = "cache_failed"

	// MetricChanged is a name of a metric to count number of cache builds that changed cached value.
	MetricChanged = "cache_changed"

	// MetricEvict is a name of metric to count evictions.
	MetricEvict = "cache_evict"
)
