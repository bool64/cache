package cache

import "context"

const (
	// MetricMiss is a name of a metric to count cache miss events.
	MetricMiss = "cache_miss"
	// MetricExpired is a name of a metric to count expired cache read events.
	MetricExpired = "cache_expired"
	// MetricHit is a name of a metric to count valid cache read events.
	MetricHit = "cache_hit"
	// MetricWrite is a name of a metric to count cache write events.
	MetricWrite = "cache_write"
	// MetricDelete is a name of a metric to count cache delete events.
	MetricDelete = "cache_delete"
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

// NewStatsTracker creates logger instance from tracking functions.
func NewStatsTracker(
	add,
	set func(ctx context.Context, name string, val float64, labelsAndValues ...string),
) StatsTracker {
	if add == nil || set == nil {
		panic("both add and set must not be nil")
	}

	return tracker{
		add: add,
		set: set,
	}
}

// StatsTracker collects incremental and absolute (gauge) metrics.
//
// This interface matches github.com/bool64/stats.Tracker.
type StatsTracker interface {
	// Add collects additional or observable value.
	Add(ctx context.Context, name string, increment float64, labelsAndValues ...string)

	// Set collects absolute value, e.g. number of cache entries at the moment.
	Set(ctx context.Context, name string, absolute float64, labelsAndValues ...string)
}

type tracker struct {
	add func(ctx context.Context, name string, val float64, labelsAndValues ...string)
	set func(ctx context.Context, name string, val float64, labelsAndValues ...string)
}

func (t tracker) Add(ctx context.Context, name string, increment float64, labelsAndValues ...string) {
	t.add(ctx, name, increment, labelsAndValues...)
}

func (t tracker) Set(ctx context.Context, name string, absolute float64, labelsAndValues ...string) {
	t.set(ctx, name, absolute, labelsAndValues...)
}
