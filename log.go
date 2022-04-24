package cache

import (
	"context"
)

// NewLogger creates logger instance from logging functions.
//
// Any logging function can be nil.
func NewLogger(
	logError,
	logWarn,
	logImportant,
	logDebug func(ctx context.Context, msg string, keysAndValues ...interface{}),
) Logger {
	return logTrait{
		logError:     logError,
		logWarn:      logWarn,
		logImportant: logImportant,
		logDebug:     logDebug,
	}
}

// Logger collects contextual information.
//
// This interface matches github.com/bool64/ctxd.Logger.
type Logger interface {
	// Error logs a message.
	Error(ctx context.Context, msg string, keysAndValues ...interface{})
}

type debugLogger interface {
	// Debug logs a message.
	Debug(ctx context.Context, msg string, keysAndValues ...interface{})
}

type warnLogger interface {
	// Warn logs a message.
	Warn(ctx context.Context, msg string, keysAndValues ...interface{})
}

type importantLogger interface {
	// Important logs an important informational message.
	Important(ctx context.Context, msg string, keysAndValues ...interface{})
}

type logFunc func(ctx context.Context, msg string, keysAndValues ...interface{})

type logTrait struct {
	logError     logFunc
	logWarn      logFunc
	logDebug     logFunc
	logImportant logFunc
}

func (lt logTrait) Error(ctx context.Context, msg string, keysAndValues ...interface{}) {
	lt.logError(ctx, msg, keysAndValues...)
}

func (lt *logTrait) setup(l Logger) {
	if l == nil {
		return
	}

	if t, ok := l.(logTrait); ok {
		*lt = t

		return
	}

	lt.logError = l.Error

	if d, ok := l.(debugLogger); ok {
		lt.logDebug = d.Debug
	}

	if w, ok := l.(warnLogger); ok {
		lt.logWarn = w.Warn
	}

	if i, ok := l.(importantLogger); ok {
		lt.logImportant = i.Important
	}
}
