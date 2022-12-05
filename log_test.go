package cache //nolint:testpackage // Testing internals.

import (
	"context"
	"testing"

	"github.com/bool64/ctxd"
	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	m := ctxd.LoggerMock{}

	l := NewLogger(m.Error, m.Warn, m.Important, m.Debug)

	ctx := context.Background()
	l.Error(ctx, "error", "foo", "bar")

	lt, ok := l.(logTrait)
	assert.True(t, ok)

	lt.logError(ctx, "error", "foo", "bar")
	lt.logWarn(ctx, "warn", "foo", "bar")
	lt.logImportant(ctx, "important", "foo", "bar")
	lt.logDebug(ctx, "debug", "foo", "bar")

	assert.Equal(t, `error: error {"foo":"bar"}
error: error {"foo":"bar"}
warn: warn {"foo":"bar"}
important: important {"foo":"bar"}
debug: debug {"foo":"bar"}
`, m.String())
}
