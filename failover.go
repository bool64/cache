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

// FailoverConfig is optional configuration for NewFailover.
type FailoverConfig struct {
	// Name is added to logs and stats.
	Name string

	// Backend is a cache instance, ShardedMap created by default.
	Backend ReadWriter

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

	// Logger collects messages with context.
	Logger ctxd.Logger

	// Stats tracks stats.
	Stats stats.Tracker

	// ObserveMutability enables deep equal check with metric collection on cache update.
	ObserveMutability bool
}

// Use is a functional option for NewFailover to apply configuration.
func (fc FailoverConfig) Use(cfg *FailoverConfig) {
	*cfg = fc
}

// Failover is a cache frontend to manage cache updates in a non-conflicting and performant way.
//
// Please use NewFailover to create instance.
type Failover struct {
	// Errors caches errors of failed updates.
	Errors *ShardedMap

	backend  ReadWriter
	lock     sync.Mutex               // Securing keyLocks
	keyLocks map[string]chan struct{} // Preventing update concurrency per key
	config   FailoverConfig
	log      ctxd.Logger
	stat     stats.Tracker
}

// NewFailover creates a Failover cache instance.
//
// Build is locked per key to avoid concurrent updates, new value is served .
// Stale value is served during non-concurrent update (up to FailoverConfig.UpdateTTL long).
func NewFailover(options ...func(cfg *FailoverConfig)) *Failover {
	cfg := FailoverConfig{}
	for _, option := range options {
		option(&cfg)
	}

	if cfg.UpdateTTL == 0 {
		cfg.UpdateTTL = time.Minute
	}

	if cfg.FailedUpdateTTL == 0 {
		cfg.FailedUpdateTTL = 20 * time.Second
	}

	f := &Failover{}
	f.config = cfg
	f.log = cfg.Logger
	f.stat = cfg.Stats
	f.backend = cfg.Backend

	if f.backend == nil {
		cfg.BackendConfig.Name = cfg.Name
		cfg.BackendConfig.Logger = cfg.Logger
		cfg.BackendConfig.Stats = cfg.Stats
		f.backend = NewShardedMap(cfg.BackendConfig.Use)
	}

	if cfg.FailedUpdateTTL > -1 {
		f.Errors = NewShardedMap(Config{
			Name:       "err_" + cfg.Name,
			Logger:     cfg.Logger,
			Stats:      cfg.Stats,
			TimeToLive: cfg.FailedUpdateTTL,

			// Short cleanup intervals to avoid storing potentially heavy errors for long time.
			DeleteExpiredAfter:       time.Minute,
			DeleteExpiredJobInterval: time.Minute,
		}.Use)
	}

	f.keyLocks = make(map[string]chan struct{})

	return f
}

// Get returns value from cache or from build function.
func (f *Failover) Get(
	ctx context.Context,
	key []byte,
	buildFunc func(ctx context.Context,
	) (interface{}, error)) (interface{}, error) {
	var (
		value interface{}
		err   error
	)

	// Performing initial check before critical section.
	if !f.config.SyncRead {
		// Checking for valid value in cache store.
		if value, err = f.backend.Read(ctx, key); err == nil {
			return value, nil
		}
	}

	// Locking key for update or finding active lock.
	f.lock.Lock()
	var keyLock chan struct{}

	alreadyLocked := false

	keyLock, alreadyLocked = f.keyLocks[string(key)]
	if !alreadyLocked {
		keyLock = make(chan struct{})
		f.keyLocks[string(key)] = keyLock
	}
	f.lock.Unlock()

	// Releasing the lock.
	defer func() {
		if !alreadyLocked {
			f.lock.Lock()
			delete(f.keyLocks, string(key))
			close(keyLock)
			f.lock.Unlock()
		}
	}()

	// Performing initial check in critical section.
	if f.config.SyncRead {
		// Checking for valid value in cache store.
		if value, err = f.backend.Read(ctx, key); err == nil {
			return value, nil
		}
	}

	// If already locked waiting for completion before checking backend again.
	if alreadyLocked {
		// Return immediately if update is in progress and stale value available.
		if val, freshEnough := f.freshEnough(err); freshEnough {
			return val, nil
		}

		return f.waitForValue(ctx, key, keyLock)
	}

	// Pushing expired value with short ttl to serve during update.
	if val, freshEnough := f.freshEnough(err); freshEnough {
		if err = f.refreshStale(ctx, key, val); err != nil {
			return nil, err
		}

		value = val
	}

	// Check if update failed recently.
	if err := f.recentlyFailed(ctx, key); err != nil {
		return nil, err
	}

	// Detaching context into background if FailoverConfig.SyncUpdate is disabled and there is a stale value already.
	ctx, syncUpdate := f.ctxSync(ctx, err)

	// Running cache build synchronously.
	if syncUpdate {
		updated, err := f.doBuild(ctx, key, value, buildFunc)
		// Return stale value if update fails.
		if err != nil {
			if f.log != nil {
				f.log.Warn(ctx, "failed to update stale cache value",
					"error", err,
					"name", f.config.Name,
					"key", key)
			}

			if value != nil {
				return value, nil
			}
		}

		return updated, err
	}

	// Disabling defer to unlock in background.
	alreadyLocked = true
	// Spawning cache update in background.
	go func() {
		defer func() {
			f.lock.Lock()
			delete(f.keyLocks, string(key))
			close(keyLock)
			f.lock.Unlock()
		}()

		_, err := f.doBuild(ctx, key, value, buildFunc)
		if err != nil && f.log != nil {
			f.log.Warn(ctx, "failed to update cache value in background",
				"error", err,
				"name", f.config.Name,
				"key", key)
		}
	}()

	return value, nil
}

func (f *Failover) freshEnough(err error) (interface{}, bool) {
	var errExpired ErrWithExpiredItem

	if errors.As(err, &errExpired) {
		if f.config.MaxStaleness == 0 || time.Since(errExpired.ExpiredAt()) < f.config.MaxStaleness {
			return errExpired.Value(), true
		}
	}

	return nil, false
}

func (f *Failover) waitForValue(ctx context.Context, key []byte, keyLock chan struct{}) (interface{}, error) {
	if f.log != nil {
		f.log.Debug(ctx, "waiting for cache value", "name", f.config.Name, "key", key)
	}

	// Waiting for value built by keyLock owner.
	<-keyLock

	// Recurse to check and return the just-updated value.
	value, err := f.backend.Read(ctx, key)
	if err == nil {
		return value, nil
	}

	var errExpired ErrWithExpiredItem
	if errors.As(err, &errExpired) {
		return errExpired.Value(), nil
	}

	if errors.Is(err, ErrNotFound) {
		// Check if update failed recently.
		if err := f.recentlyFailed(ctx, key); err != nil {
			return nil, err
		}
	}

	return value, err
}

func (f *Failover) refreshStale(ctx context.Context, key []byte, value interface{}) error {
	if f.log != nil {
		f.log.Debug(ctx, "refreshing expired value",
			"name", f.config.Name,
			"key", key,
			"value", value)
	}

	if f.stat != nil {
		f.stat.Add(ctx, MetricRefreshed, 1, "name", f.config.Name)
	}

	writeErr := f.backend.Write(WithTTL(ctx, f.config.UpdateTTL, false), key, value)
	if writeErr != nil {
		return ctxd.WrapError(ctx, writeErr, "failed to refresh expired value")
	}

	return nil
}

func (f *Failover) doBuild(
	ctx context.Context,
	key []byte,
	value interface{},
	buildFunc func(ctx context.Context) (interface{}, error),
) (interface{}, error) {
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

		return nil, err
	}

	writeErr := f.backend.Write(ctx, key, uVal)
	if writeErr != nil {
		return nil, writeErr
	}

	if f.config.ObserveMutability && value != nil {
		f.observeMutability(ctx, uVal, value)
	}

	return uVal, err
}

func (f *Failover) ctxSync(ctx context.Context, err error) (context.Context, bool) {
	syncUpdate := f.config.SyncUpdate || err != nil
	if syncUpdate {
		return ctx, true
	}

	// Detaching context for async update.
	return detachedContext{ctx}, false
}

func (f *Failover) recentlyFailed(ctx context.Context, key []byte) error {
	if f.config.FailedUpdateTTL > -1 {
		errVal, err := f.Errors.Read(ctx, key)
		if err == nil {
			return errVal.(error)
		}
	}

	return nil
}

func (f *Failover) observeMutability(ctx context.Context, uVal, value interface{}) {
	equal := reflect.DeepEqual(value, uVal)
	if !equal {
		f.stat.Add(ctx, MetricChanged, 1, "name", f.config.Name)
	}
}
