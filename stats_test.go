package cache_test

import (
	"context"
	"testing"

	"github.com/bool64/cache"
	"github.com/bool64/stats"
	"github.com/stretchr/testify/assert"
)

func TestNewStatsTracker(t *testing.T) {
	m := stats.TrackerMock{}
	s := cache.NewStatsTracker(m.Add, m.Set)
	ctx := context.Background()

	s.Set(ctx, "foo_set", 123, "foo", "bar")
	s.Add(ctx, "foo_add", 123, "foo", "bar")

	assert.Equal(t, map[string]float64{"foo_add{foo=\"bar\"}": 123, "foo_set{foo=\"bar\"}": 123}, m.LabeledValues())
}
