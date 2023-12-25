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
	deleters map[string][]Deleter // By name.

	mu                sync.Mutex
	labeledKeysByName map[string]map[string][]string
}

// NewInvalidationIndex creates new instance of label-based invalidator.
func NewInvalidationIndex(deleters ...Deleter) *InvalidationIndex {
	ds := make(map[string][]Deleter)

	if len(deleters) > 0 {
		ds["default"] = deleters
	}

	return &InvalidationIndex{
		deleters:          ds,
		labeledKeysByName: make(map[string]map[string][]string), // name -> labeledKeys
	}
}

// AddCache adds a named instance of cache with deletable entries.
func (i *InvalidationIndex) AddCache(name string, deleter Deleter) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.deleters[name] = []Deleter{deleter}
}

// AddInvalidationLabels registers invalidation labels to a cache key in default cache.
func (i *InvalidationIndex) AddInvalidationLabels(key []byte, labels ...string) {
	i.AddLabels("default", key, labels...)
}

// AddLabels registers invalidation labels to a cache key.
func (i *InvalidationIndex) AddLabels(cacheName string, key []byte, labels ...string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	labeledKeys := i.labeledKeysByName[cacheName]
	if labeledKeys == nil {
		labeledKeys = make(map[string][]string)
		i.labeledKeysByName[cacheName] = labeledKeys
	}

	ks := string(key)
	for _, label := range labels {
		labeledKeys[label] = append(labeledKeys[label], ks)
	}
}

// InvalidateByLabels deletes keys from cache that have any of provided labels and returns number of deleted entries.
// If delete fails, function puts unprocessed keys back in the index and returns.
func (i *InvalidationIndex) InvalidateByLabels(ctx context.Context, labels ...string) (int, error) {
	i.mu.Lock()

	labeledKeysByName := make(map[string]map[string][]string)
	deleters := make(map[string][]Deleter)

	for name, labeledKeys := range i.labeledKeysByName {
		labeledKeysByName[name] = labeledKeys
		deleters[name] = i.deleters[name]
	}

	i.mu.Unlock()

	cnt := 0

	for name, labeledKeys := range i.labeledKeysByName {
		n, err := i.invalidateByLabels(ctx, labeledKeys, deleters[name], labels...)
		cnt += n

		if err != nil {
			return cnt, err
		}
	}

	return cnt, nil
}

func (i *InvalidationIndex) invalidateByLabels(ctx context.Context, labeledKeys map[string][]string, deleters []Deleter, labels ...string) (int, error) {
	cutKeys := i.cutKeys(labeledKeys, labels...)
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

				labeledKeys[label] = append(labeledKeys[label], keys...)
			}
		}
	}()

	cnt := 0

	for _, label := range labels {
		for _, k := range cutKeys[label] {
			if deleted[k] {
				continue
			}

			for _, d := range deleters {
				err := d.Delete(ctx, []byte(k))
				if err != nil {
					if !errors.Is(err, ErrNotFound) {
						return cnt, err
					}
				} else {
					cnt++
				}
			}

			deleted[k] = true
		}

		delete(cutKeys, label)
	}

	return cnt, nil
}

func (i *InvalidationIndex) cutKeys(labeledKeys map[string][]string, labels ...string) map[string][]string {
	res := make(map[string][]string, len(labels))

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, label := range labels {
		res[label] = labeledKeys[label]
		delete(labeledKeys, label)
	}

	return res
}
