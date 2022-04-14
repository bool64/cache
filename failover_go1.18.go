//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
)

// FailoverConfigOf is optional configuration for NewFailoverOf.
type FailoverConfigOf[V any] struct {
	// Name is added to logs and stats.
	Name string

	// Backend is a cache instance, ShardedMap created by default.
	Backend ReadWriterOf[V]

	// BackendConfig is a configuration for ShardedMap cache instance if Backend is not provided.
	BackendConfig Config

	// FailedUpdateTTL is ttl of failed build cache, default 20s, -1 disables errors cache.
	FailedUpdateTTL time.Duration

	// UpdateTTL is a time interval to retry update, default 1 minute.
	UpdateTTL time.Duration

	// SyncUpdate disables update in background, default is background update with stale value served.
	SyncUpdate bool

	// SyncRead enables backend reading in the critical section to ensure cache miss
	// will not trigger multiple updates sequentially.
	//
	// Probability of such issue is low, there is performance penalty for enabling this option.
	SyncRead bool

	// MaxStaleness is duration when value can be served after expiration.
	// If value has expired longer than this duration it won't be served unless value update failure.
	MaxStaleness time.Duration

	// FailHard disables serving of stale value in case up update failure.
	FailHard bool

	// Logger collects messages with context.
	Logger ctxd.Logger

	// Stats tracks stats.
	Stats stats.Tracker

	// ObserveMutability enables deep equal check with metric collection on cache update.
	ObserveMutability bool
}

// Use is a functional option for NewFailover to apply configuration.
func (fc FailoverConfigOf[V]) Use(cfg *FailoverConfigOf[V]) {
	*cfg = fc
}

// FailoverOf is a cache frontend to manage cache updates in a non-conflicting and performant way.
//
// Please use NewFailoverOf to create instance.
type FailoverOf[V any] struct {
	// Errors caches errors of failed updates.
	Errors *ShardedMapOf[error]

	backend  ReadWriterOf[V]
	lock     sync.Mutex          // Securing keyLocks
	keyLocks map[string]*klOf[V] // Preventing update concurrency per key
	config   FailoverConfigOf[V]
	log      ctxd.Logger
	stat     stats.Tracker
}

// NewFailoverOf creates a FailoverOf cache instance.
//
// Build is locked per key to avoid concurrent updates, new value is served .
// Stale value is served during non-concurrent update (up to FailoverConfigOf.UpdateTTL long).
func NewFailoverOf[V any](options ...func(cfg *FailoverConfigOf[V])) *FailoverOf[V] {
	cfg := FailoverConfigOf[V]{}
	for _, option := range options {
		option(&cfg)
	}

	if cfg.UpdateTTL == 0 {
		cfg.UpdateTTL = time.Minute
	}

	if cfg.FailedUpdateTTL == 0 {
		cfg.FailedUpdateTTL = 20 * time.Second
	}

	f := &FailoverOf[V]{}
	f.config = cfg
	f.log = cfg.Logger
	f.stat = cfg.Stats
	f.backend = cfg.Backend

	if f.backend == nil {
		cfg.BackendConfig.Name = cfg.Name
		cfg.BackendConfig.Logger = cfg.Logger
		cfg.BackendConfig.Stats = cfg.Stats
		f.backend = NewShardedMapOf[V](cfg.BackendConfig.Use)
	}

	if cfg.FailedUpdateTTL > -1 {
		f.Errors = NewShardedMapOf[error](Config{
			Name:       "err_" + cfg.Name,
			Logger:     cfg.Logger,
			Stats:      cfg.Stats,
			TimeToLive: cfg.FailedUpdateTTL,

			// Short cleanup intervals to avoid storing potentially heavy errors for long time.
			DeleteExpiredAfter:       time.Minute,
			DeleteExpiredJobInterval: time.Minute,
		}.Use)
	}

	f.keyLocks = make(map[string]*klOf[V])

	return f
}

// Get returns value from cache or from build function.
func (f *FailoverOf[V]) Get(
	ctx context.Context,
	key []byte,
	buildFunc func(ctx context.Context) (V, error),
) (V, error) {
	var (
		val V
		err error
	)

	// Performing initial check before critical section.
	if !f.config.SyncRead {
		// Checking for valid value in cache store.
		if val, err = f.backend.Read(ctx, key); err == nil {
			return val, nil
		}
	}

	// Locking key for update or finding active lock.
	f.lock.Lock()
	var keyLock *klOf[V]

	alreadyLocked := false

	keyLock, alreadyLocked = f.keyLocks[string(key)]
	if !alreadyLocked {
		keyLock = &klOf[V]{lock: make(chan struct{})}
		f.keyLocks[string(key)] = keyLock
	}
	f.lock.Unlock()

	// Releasing the lock.
	defer func() {
		if !alreadyLocked {
			f.lock.Lock()
			delete(f.keyLocks, string(key))
			close(keyLock.lock)
			f.lock.Unlock()
		}
	}()

	// Performing initial check in critical section.
	if f.config.SyncRead {
		// Checking for valid value in cache store.
		if val, err = f.backend.Read(ctx, key); err == nil {
			if !alreadyLocked {
				keyLock.val = val
			}

			return val, nil
		}
	}

	// If already locked waiting for completion before checking backend again.
	if alreadyLocked {
		// Return immediately if update is in progress and stale value available.
		if val, freshEnough := f.freshEnough(err); freshEnough {
			return val, nil
		}

		return f.waitForValue(withoutSkipRead(ctx), key, keyLock)
	}

	// Pushing expired value with short ttl to serve during update.
	if v, freshEnough := f.freshEnough(err); freshEnough {
		if err = f.refreshStale(ctx, key, v); err != nil {
			return val, err
		}

		val = v
	}

	// Check if update failed recently.
	if err := f.recentlyFailed(ctx, key); err != nil {
		keyLock.err = err

		return val, err
	}

	// Detaching context into background if FailoverConfig.SyncUpdate is disabled and there is a stale value already.
	ctx, syncUpdate := f.ctxSync(ctx, err)

	// Running cache build synchronously.
	if syncUpdate {
		keyLock.val, keyLock.err = f.doBuild(ctx, key, val, buildFunc)
		// Return stale value if update fails.
		if keyLock.err != nil {
			if f.log != nil {
				f.log.Warn(ctx, "failed to update stale cache value",
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

	// Disabling defer to unlock in background.
	alreadyLocked = true
	// Spawning cache update in background.
	go func() {
		defer func() {
			f.lock.Lock()
			delete(f.keyLocks, string(key))
			close(keyLock.lock)
			f.lock.Unlock()
		}()

		keyLock.val, keyLock.err = f.doBuild(ctx, key, val, buildFunc)
		if keyLock.err != nil && f.log != nil {
			f.log.Warn(ctx, "failed to update cache value in background",
				"error", keyLock.err,
				"name", f.config.Name,
				"key", key)
		}
	}()

	return val, nil
}

type klOf[V any] struct {
	val  V
	err  error
	lock chan struct{}
}

func (f *FailoverOf[V]) freshEnough(err error) (val V, _ bool) {
	var errExpired ErrWithExpiredItemOf[V]

	if errors.As(err, &errExpired) {
		if f.config.MaxStaleness == 0 || time.Since(errExpired.ExpiredAt()) < f.config.MaxStaleness {
			return errExpired.Value(), true
		}
	}

	return val, false
}

func (f *FailoverOf[V]) waitForValue(ctx context.Context, key []byte, keyLock *klOf[V]) (V, error) {
	if f.log != nil {
		f.log.Debug(ctx, "waiting for cache value", "name", f.config.Name, "key", key)
	}

	// Waiting for value built by keyLock owner.
	<-keyLock.lock

	return keyLock.val, keyLock.err
}

func (f *FailoverOf[V]) refreshStale(ctx context.Context, key []byte, val V) error {
	if f.log != nil {
		f.log.Debug(ctx, "refreshing expired value",
			"name", f.config.Name,
			"key", key,
			"value", val)
	}

	if f.stat != nil {
		f.stat.Add(ctx, MetricRefreshed, 1, "name", f.config.Name)
	}

	writeErr := f.backend.Write(WithTTL(ctx, f.config.UpdateTTL, false), key, val)
	if writeErr != nil {
		return ctxd.WrapError(ctx, writeErr, "failed to refresh expired value")
	}

	return nil
}

func (f *FailoverOf[V]) doBuild(
	ctx context.Context,
	key []byte,
	val V,
	buildFunc func(ctx context.Context) (V, error),
) (v V, _ error) { // nolint:dupl // Generic API is different.
	if f.stat != nil {
		defer func() {
			f.stat.Add(ctx, MetricBuild, 1, "name", f.config.Name)
		}()
	}

	if f.log != nil {
		f.log.Debug(ctx, "building cache value", "name", f.config.Name, "key", key)
	}

	uVal, err := buildFunc(ctx)
	if err != nil {
		if f.stat != nil {
			f.stat.Add(ctx, MetricFailed, 1, "name", f.config.Name)
		}

		if f.config.FailedUpdateTTL > -1 {
			writeErr := f.Errors.Write(ctx, key, err)
			if writeErr != nil && f.log != nil {
				f.log.Error(ctx, "failed to cache update failure",
					"error", writeErr,
					"updateErr", err,
					"key", key,
					"name", f.config.Name)
			}
		}

		return v, err
	}

	writeErr := f.backend.Write(ctx, key, uVal)
	if writeErr != nil {
		return v, writeErr
	}

	if f.config.ObserveMutability && err == nil {
		f.observeMutability(ctx, uVal, val)
	}

	return uVal, err
}

func (f *FailoverOf[V]) ctxSync(ctx context.Context, err error) (context.Context, bool) {
	syncUpdate := f.config.SyncUpdate || err != nil
	if syncUpdate {
		return ctx, true
	}

	// Detaching context for async update.
	return detachedContext{ctx}, false
}

func (f *FailoverOf[V]) recentlyFailed(ctx context.Context, key []byte) error {
	if f.config.FailedUpdateTTL > -1 {
		errVal, err := f.Errors.Read(ctx, key)
		if err == nil {
			return errVal
		}
	}

	return nil
}

func (f *FailoverOf[V]) observeMutability(ctx context.Context, uVal, val V) {
	equal := reflect.DeepEqual(val, uVal)
	if !equal {
		f.stat.Add(ctx, MetricChanged, 1, "name", f.config.Name)
	}
}
