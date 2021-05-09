package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Invalidator is a registry of cache expiration triggers.
type Invalidator struct {
	sync.Mutex

	// SkipInterval defines minimal duration between two cache invalidations (flood protection).
	SkipInterval time.Duration

	// Callbacks contains a list of functions to call on invalidate.
	Callbacks []func(ctx context.Context)

	lastRun time.Time
}

// Invalidate triggers cache expiration.
func (i *Invalidator) Invalidate(ctx context.Context) error {
	if i.Callbacks == nil {
		return ErrNothingToInvalidate
	}

	i.Lock()
	defer i.Unlock()

	if i.SkipInterval == 0 {
		i.SkipInterval = 15 * time.Second
	}

	if time.Since(i.lastRun) < i.SkipInterval {
		return fmt.Errorf("%w at %s, %s did not pass",
			ErrAlreadyInvalidated, i.lastRun.String(), i.SkipInterval.String())
	}

	i.lastRun = time.Now()
	for _, cb := range i.Callbacks {
		cb(ctx)
	}

	return nil
}
