//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// FailoverConfigBy is optional configuration for NewFailoverBy.
type FailoverConfigBy[K comparable, V any] struct {
	// Name is added to logs and stats.
	Name string

	// Backend is a cache instance, ShardedMapBy created by default.
	Backend ReadWriterBy[K, V]

	// BackendConfig is a configuration for ShardedMapBy cache instance if Backend is not provided.
	BackendConfig ConfigBy[K]

	// FailedUpdateTTL is ttl of failed build cache, default 20s, -1 disables errors cache.
	FailedUpdateTTL time.Duration

	// UpdateTTL is a time interval to retry update, default 1 minute.
	UpdateTTL time.Duration

	// SyncUpdate disables update in background, default is background update with stale value served.
	SyncUpdate bool

	// SyncRead enables backend reading in the critical section to ensure cache miss
	// will not trigger multiple updates sequentially.
	SyncRead bool

	// MaxStaleness is duration when value can be served after expiration.
	MaxStaleness time.Duration

	// FailHard disables serving of stale value in case up update failure.
	FailHard bool

	// Logger collects messages with context.
	Logger Logger

	// Stats tracks stats.
	Stats StatsTracker

	// ObserveMutability enables deep equal check with metric collection on cache update.
	ObserveMutability bool
}

// Use is a functional option for NewFailoverBy to apply configuration.
func (fc FailoverConfigBy[K, V]) Use(cfg *FailoverConfigBy[K, V]) {
	*cfg = fc
}

// FailoverBy is a cache frontend to manage typed-key cache updates in a non-conflicting and performant way.
type FailoverBy[K comparable, V any] struct {
	// Errors caches errors of failed updates.
	Errors *ShardedMapBy[K, error]

	backend ReadWriterBy[K, V]
	wr      WriteAndReaderBy[K, V]

	lock     sync.Mutex
	keyLocks map[K]*klBy[V]
	config   FailoverConfigBy[K, V]
	logTrait
	stat StatsTracker
}

// NewFailoverBy creates a FailoverBy cache instance.
func NewFailoverBy[K comparable, V any](options ...func(cfg *FailoverConfigBy[K, V])) *FailoverBy[K, V] {
	cfg := FailoverConfigBy[K, V]{}
	for _, option := range options {
		option(&cfg)
	}

	if cfg.UpdateTTL == 0 {
		cfg.UpdateTTL = time.Minute
	}

	if cfg.FailedUpdateTTL == 0 {
		cfg.FailedUpdateTTL = 20 * time.Second
	}

	f := &FailoverBy[K, V]{}
	f.config = cfg
	f.setup(cfg.Logger)
	f.stat = cfg.Stats
	f.backend = cfg.Backend

	if f.backend == nil {
		cfg.BackendConfig.Name = cfg.Name
		cfg.BackendConfig.Logger = cfg.Logger
		cfg.BackendConfig.Stats = cfg.Stats
		f.backend = NewShardedMapBy[K, V](cfg.BackendConfig.Use)
	}

	f.wr, _ = f.backend.(WriteAndReaderBy[K, V])

	if cfg.FailedUpdateTTL > -1 {
		f.Errors = NewShardedMapBy[K, error](ConfigBy[K]{
			Config: Config{
				Name:       "err_" + cfg.Name,
				Logger:     cfg.Logger,
				Stats:      cfg.Stats,
				TimeToLive: cfg.FailedUpdateTTL,

				DeleteExpiredAfter:       time.Minute,
				DeleteExpiredJobInterval: time.Minute,
			},
			ShardFunc: cfg.BackendConfig.ShardFunc,
		}.Use)
	}

	f.keyLocks = make(map[K]*klBy[V])

	return f
}

// Get returns value from cache or from build function.
func (f *FailoverBy[K, V]) Get(
	ctx context.Context,
	key K,
	buildFunc func(ctx context.Context) (V, error),
) (V, error) {
	var (
		val V
		err error
	)

	if !f.config.SyncRead {
		if val, err = f.backend.Read(ctx, key); err == nil {
			return val, nil
		}
	}

	f.lock.Lock()
	var keyLock *klBy[V]

	alreadyLocked := false

	keyLock, alreadyLocked = f.keyLocks[key]
	if !alreadyLocked {
		keyLock = &klBy[V]{lock: make(chan struct{})}
		f.keyLocks[key] = keyLock
	}
	f.lock.Unlock()

	defer func() {
		if !alreadyLocked {
			f.lock.Lock()
			delete(f.keyLocks, key)
			close(keyLock.lock)
			f.lock.Unlock()
		}
	}()

	if f.config.SyncRead {
		if val, err = f.backend.Read(ctx, key); err == nil {
			if !alreadyLocked {
				keyLock.val = val
			}

			return val, nil
		}
	}

	if alreadyLocked {
		if val, freshEnough := f.freshEnough(err); freshEnough {
			return val, nil
		}

		return f.waitForValue(withoutSkipRead(ctx), key, keyLock)
	}

	if v, freshEnough := f.freshEnough(err); freshEnough {
		if err = f.refreshStale(ctx, key, v); err != nil {
			return val, err
		}

		val = v
	}

	if err := f.recentlyFailed(ctx, key); err != nil {
		keyLock.err = err

		return val, err
	}

	ctx, syncUpdate := f.ctxSync(ctx, err)

	if syncUpdate {
		keyLock.val, keyLock.err = f.doBuild(ctx, key, val, !errors.Is(err, ErrNotFound), buildFunc)
		if keyLock.err != nil {
			if f.logWarn != nil {
				f.logWarn(ctx, "failed to update stale cache value",
					"error", keyLock.err,
					"name", f.config.Name,
					"key", key)
			}

			if !f.config.FailHard && !errors.Is(err, ErrNotFound) {
				return val, nil
			}
		}

		return keyLock.val, keyLock.err
	}

	alreadyLocked = true
	runBuild := func() {
		defer func() {
			f.lock.Lock()
			delete(f.keyLocks, key)
			close(keyLock.lock)
			f.lock.Unlock()
		}()

		keyLock.val, keyLock.err = f.doBuild(ctx, key, val, !errors.Is(err, ErrNotFound), buildFunc)
		if keyLock.err != nil && f.logWarn != nil {
			f.logWarn(ctx, "failed to update cache value in background",
				"error", keyLock.err,
				"name", f.config.Name,
				"key", key)
		}
	}

	go runBuild()

	return val, nil
}

type klBy[V any] struct {
	val  V
	err  error
	lock chan struct{}
}

func (f *FailoverBy[K, V]) freshEnough(err error) (val V, _ bool) {
	var errExpired ErrWithExpiredItemOf[V]

	if errors.As(err, &errExpired) {
		if f.config.MaxStaleness == 0 || time.Since(errExpired.ExpiredAt()) < f.config.MaxStaleness {
			return errExpired.Value(), true
		}
	}

	return val, false
}

func (f *FailoverBy[K, V]) waitForValue(ctx context.Context, key K, keyLock *klBy[V]) (V, error) {
	if f.logDebug != nil {
		f.logDebug(ctx, "waiting for cache value", "name", f.config.Name, "key", key)
	}

	<-keyLock.lock

	return keyLock.val, keyLock.err
}

func (f *FailoverBy[K, V]) refreshStale(ctx context.Context, key K, val V) error {
	if f.logDebug != nil {
		f.logDebug(ctx, "refreshing expired value",
			"name", f.config.Name,
			"key", key,
			"value", val)
	}

	if f.stat != nil {
		f.stat.Add(ctx, MetricRefreshed, 1, "name", f.config.Name)
	}

	writeErr := f.backend.Write(WithTTL(ctx, f.config.UpdateTTL, false), key, val)
	if writeErr != nil {
		return fmt.Errorf("failed to refresh expired value: %w", writeErr)
	}

	return nil
}

func (f *FailoverBy[K, V]) doBuild(
	ctx context.Context,
	key K,
	val V,
	hasPreviousValue bool,
	buildFunc func(ctx context.Context) (V, error),
) (v V, _ error) {
	if f.stat != nil {
		defer func() {
			f.stat.Add(ctx, MetricBuild, 1, "name", f.config.Name)
		}()
	}

	if f.logDebug != nil {
		f.logDebug(ctx, "building cache value", "name", f.config.Name, "key", key)
	}

	uVal, err := buildFunc(ctx)
	if err != nil {
		if f.stat != nil {
			f.stat.Add(ctx, MetricFailed, 1, "name", f.config.Name)
		}

		if f.config.FailedUpdateTTL > -1 {
			writeErr := f.Errors.Write(ctx, key, err)
			if writeErr != nil && f.logError != nil {
				f.logError(ctx, "failed to cache update failure",
					"error", writeErr,
					"updateErr", err,
					"key", key,
					"name", f.config.Name)
			}
		}

		return v, err
	}

	if f.wr != nil {
		uVal, err = f.wr.WriteAndRead(ctx, key, uVal)
		if err != nil {
			return v, err
		}
	} else {
		writeErr := f.backend.Write(ctx, key, uVal)
		if writeErr != nil {
			return v, writeErr
		}
	}

	if f.config.ObserveMutability && hasPreviousValue {
		f.observeMutability(ctx, uVal, val)
	}

	return uVal, err
}

func (f *FailoverBy[K, V]) ctxSync(ctx context.Context, err error) (context.Context, bool) {
	syncUpdate := f.config.SyncUpdate || err != nil
	if syncUpdate {
		return ctx, true
	}

	return detachedContext{ctx}, false
}

func (f *FailoverBy[K, V]) recentlyFailed(ctx context.Context, key K) error {
	if f.config.FailedUpdateTTL > -1 {
		errVal, err := f.Errors.Read(ctx, key)
		if err == nil {
			return errVal
		}
	}

	return nil
}

func (f *FailoverBy[K, V]) observeMutability(ctx context.Context, uVal, val V) {
	equal := reflect.DeepEqual(val, uVal)
	if !equal {
		f.stat.Add(ctx, MetricChanged, 1, "name", f.config.Name)
	}
}
