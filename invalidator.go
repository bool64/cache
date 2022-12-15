package cache

import (
	"context"
	"errors"
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

// InvalidationIndex keeps index of keys labeled for future invalidation.
type InvalidationIndex struct {
	deleter Deleter

	mu          sync.Mutex
	labeledKeys map[string][]string
}

// NewInvalidationIndex creates new instance of label-based invalidator.
func NewInvalidationIndex(deleter Deleter) *InvalidationIndex {
	return &InvalidationIndex{
		deleter:     deleter,
		labeledKeys: make(map[string][]string),
	}
}

// AddInvalidationLabels registers invalidation labels to a cache key.
func (i *InvalidationIndex) AddInvalidationLabels(key []byte, labels ...string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	ks := string(key)
	for _, label := range labels {
		i.labeledKeys[label] = append(i.labeledKeys[label], ks)
	}
}

// InvalidateByLabels deletes keys from cache that have any of provided labels and returns number of deleted entries.
// If delete fails, function puts unprocessed keys back in the index and returns.
func (i *InvalidationIndex) InvalidateByLabels(ctx context.Context, labels ...string) (int, error) {
	cutKeys := i.cutKeys(labels...)
	deleted := make(map[string]bool) // Deduplication index to avoid multiple deletes for keys with multiple labels.

	defer func() {
		// Return unprocessed keys back.
		if len(cutKeys) > 0 {
			i.mu.Lock()
			defer i.mu.Unlock()

			for label, keys := range cutKeys {
				// Cut keys already deleted in other labels.
				for j, k := range keys {
					if deleted[k] {
						keys[j] = keys[len(keys)-1]
						keys = keys[:len(keys)-1]
					}
				}

				i.labeledKeys[label] = append(i.labeledKeys[label], keys...)
			}
		}
	}()

	cnt := 0

	for _, label := range labels {
		for _, k := range cutKeys[label] {
			if deleted[k] {
				continue
			}

			err := i.deleter.Delete(ctx, []byte(k))
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}

				return cnt, err
			}

			deleted[k] = true

			cnt++
		}

		delete(cutKeys, label)
	}

	return cnt, nil
}

func (i *InvalidationIndex) cutKeys(labels ...string) map[string][]string {
	res := make(map[string][]string, len(labels))

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, label := range labels {
		res[label] = i.labeledKeys[label]
		delete(i.labeledKeys, label)
	}

	return res
}
