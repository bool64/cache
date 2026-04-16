//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swaggest/assertjson"
)

func backendsBy[V any](options ...func(*cache.Config)) []interface {
	cache.ReadWriterBy[string, V]
	Len() int
} {
	sharded := cache.NewShardedMapBy[string, V](func(cfg *cache.ConfigBy[string]) {
		for _, option := range options {
			option(&cfg.Config)
		}
	})

	return []interface {
		cache.ReadWriterBy[string, V]
		Len() int
	}{
		sharded,
		cache.NewSyncMapBy[string, V](options...),
	}
}

type expireAllBy interface {
	ExpireAll(ctx context.Context)
}

func TestNewShardedMapBy(t *testing.T) {
	logger := ctxd.LoggerMock{}
	st := stats.TrackerMock{}

	func() {
		c := cache.NewShardedMapBy[string, string](func(config *cache.ConfigBy[string]) {
			config.Logger = &logger
			config.Stats = &st
			config.Name = "test"
			config.TimeToLive = time.Hour
		})

		ctx := context.Background()
		assert.NoError(t, c.Write(ctx, "foo", "bar"))
		v, err := c.Read(ctx, "foo")
		assert.NoError(t, err)
		assert.Equal(t, "bar", v)
		assert.Equal(t, 1, c.Len())

		c.ExpireAll(ctx)

		v, err = c.Read(ctx, "foo")
		assert.EqualError(t, err, cache.ErrExpired.Error())
		assert.Empty(t, v)
		assert.Equal(t, "bar", err.(cache.ErrWithExpiredItemOf[string]).Value())

		assert.NoError(t, c.Delete(ctx, "foo"))
		assert.Equal(t, 0, c.Len())

		c.DeleteAll(ctx)
	}()

	runtime.GC()
	runtime.GC()

	logger.Lock()
	loggedEntries := logger.LoggedEntries
	logger.Unlock()

	assertjson.EqualMarshal(t, []byte(`[
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"wrote to cache",
		"data":{"key":"foo","name":"test","ttl":"<ignore-diff>","value":"bar"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache hit",
		"data":{
		  "entry":{"key":"foo","val":"bar","exp":"<ignore-diff>"},
		  "name":"test"
		}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"expired all entries in cache",
		"data":{"count":1,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache key expired","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"deleted cache entry","data":{"key":"foo","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"deleted all entries in cache",
		"data":{"count":0,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  }
	]`), loggedEntries)

	assert.Equal(t, `cache_delete{name="test"} 1
cache_expired{name="test"} 2
cache_hit{name="test"} 1
cache_items{name="test"} 0
cache_write{name="test"} 1`, st.Metrics())
}

func TestNewShardedMapBy_noExpiration(t *testing.T) {
	logger := ctxd.LoggerMock{}
	st := stats.TrackerMock{}

	func() {
		c := cache.NewShardedMapBy[string, string](func(config *cache.ConfigBy[string]) {
			config.Logger = &logger
			config.Stats = &st
			config.Name = "test"
			config.TimeToLive = cache.UnlimitedTTL
		})

		ctx := context.Background()
		assert.NoError(t, c.Write(ctx, "foo", "bar"))
		v, err := c.Read(ctx, "foo")
		assert.NoError(t, err)
		assert.Equal(t, "bar", v)
		assert.Equal(t, 1, c.Len())

		c.ExpireAll(ctx)

		v, err = c.Read(ctx, "foo")
		assert.EqualError(t, err, cache.ErrExpired.Error())
		assert.Empty(t, v)
		assert.Equal(t, "bar", err.(cache.ErrWithExpiredItemOf[string]).Value())

		assert.NoError(t, c.Delete(ctx, "foo"))
		assert.Equal(t, 0, c.Len())

		c.DeleteAll(ctx)
	}()

	runtime.GC()
	runtime.GC()

	logger.Lock()
	loggedEntries := logger.LoggedEntries
	logger.Unlock()

	assertjson.EqualMarshal(t, []byte(`[
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"wrote to cache",
		"data":{"key":"foo","name":"test","ttl":"<ignore-diff>","value":"bar"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache hit",
		"data":{
		  "entry":{"key":"foo","val":"bar","exp":"<ignore-diff>"},
		  "name":"test"
		}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"expired all entries in cache",
		"data":{"count":1,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache key expired","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"deleted cache entry","data":{"key":"foo","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"deleted all entries in cache",
		"data":{"count":0,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  }
	]`), loggedEntries)

	assert.Equal(t, `cache_delete{name="test"} 1
cache_expired{name="test"} 2
cache_hit{name="test"} 1
cache_items{name="test"} 0
cache_write{name="test"} 1`, st.Metrics())
}

func TestNewShardedMapBy_Load_Store(t *testing.T) {
	c := cache.NewShardedMapBy[string, string](func(config *cache.ConfigBy[string]) {
		config.Name = "test"
		config.TimeToLive = time.Hour
	})

	c.Store("foo", "bar")
	v, loaded := c.Load("foo")
	assert.True(t, loaded)
	assert.Equal(t, "bar", v)
	assert.Equal(t, 1, c.Len())
}

func TestNewSyncMapBy(t *testing.T) {
	logger := ctxd.LoggerMock{}
	st := stats.TrackerMock{}

	func() {
		c := cache.NewSyncMapBy[string, string](func(config *cache.Config) {
			config.Logger = &logger
			config.Stats = &st
			config.Name = "test"
			config.TimeToLive = time.Hour
		})

		ctx := context.Background()
		assert.NoError(t, c.Write(ctx, "foo", "bar"))
		v, err := c.Read(ctx, "foo")
		assert.NoError(t, err)
		assert.Equal(t, "bar", v)
		assert.Equal(t, 1, c.Len())

		c.ExpireAll(ctx)

		v, err = c.Read(ctx, "foo")
		assert.EqualError(t, err, cache.ErrExpired.Error())
		assert.Empty(t, v)
		assert.Equal(t, "bar", err.(cache.ErrWithExpiredItemOf[string]).Value())

		assert.NoError(t, c.Delete(ctx, "foo"))
		assert.Equal(t, 0, c.Len())

		c.DeleteAll(ctx)
	}()

	runtime.GC()
	runtime.GC()

	logger.Lock()
	loggedEntries := logger.LoggedEntries
	logger.Unlock()

	assertjson.EqualMarshal(t, []byte(`[
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"wrote to cache",
		"data":{"key":"foo","name":"test","ttl":"<ignore-diff>","value":"bar"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache hit",
		"data":{
		  "entry":{"key":"foo","val":"bar","exp":"<ignore-diff>"},
		  "name":"test"
		}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"expired all entries in cache",
		"data":{"count":1,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"cache key expired","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"deleted cache entry","data":{"key":"foo","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"important",
		"message":"deleted all entries in cache",
		"data":{"count":0,"elapsed":"<ignore-diff>","name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  },
	  {
		"time":"<ignore-diff>","level":"debug",
		"message":"<ignore-diff>","data":{"name":"test"}
	  }
	]`), loggedEntries)

	assert.Equal(t, `cache_delete{name="test"} 1
cache_expired{name="test"} 2
cache_hit{name="test"} 1
cache_items{name="test"} 0
cache_write{name="test"} 1`, st.Metrics())
}

func TestNoOpBy(t *testing.T) {
	v, err := (cache.NoOpBy[string, int]{}).Read(context.Background(), "foo")
	assert.Equal(t, 0, v)
	assert.EqualError(t, err, cache.ErrNotFound.Error())

	err = (cache.NoOpBy[string, int]{}).Write(context.Background(), "foo", 123)
	assert.NoError(t, err)

	v, err = (cache.NoOpBy[string, int]{}).Read(context.Background(), "foo")
	assert.Equal(t, 0, v)
	assert.EqualError(t, err, cache.ErrNotFound.Error())

	err = (cache.NoOpBy[string, int]{}).Delete(context.Background(), "foo")
	assert.EqualError(t, err, cache.ErrNotFound.Error())
}

func TestNewShardedMapBy_customSharder(t *testing.T) {
	type key struct {
		ID int
	}

	c := cache.NewShardedMapBy[key, string](func(cfg *cache.ConfigBy[key]) {
		cfg.TimeToLive = time.Hour
		cfg.ShardFunc = func(k key) uint64 {
			return uint64(k.ID)
		}
	})

	ctx := context.Background()
	require.NoError(t, c.Write(ctx, key{ID: 7}, "bar"))

	v, err := c.Read(ctx, key{ID: 7})
	require.NoError(t, err)
	assert.Equal(t, "bar", v)
}

func TestNewShardedMapBy_unsupportedKeyPanics(t *testing.T) {
	type key struct {
		ID int
	}

	assert.PanicsWithValue(t,
		"cache: no default key sharder for K=cache_test.key; configure ShardFunc in keyed config/options",
		func() {
			_ = cache.NewShardedMapBy[key, string]()
		},
	)
}

func TestFailoverBy_Get(t *testing.T) {
	f := cache.NewFailoverBy[string, string]()

	s, err := f.Get(context.Background(), "aaaa", func(ctx context.Context) (string, error) {
		return "bbb", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "bbb", s)
}

func TestFailoverBy_Get_ValueConcurrency(t *testing.T) {
	for _, be := range backendsBy[int]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			numKeys := 10
			concurrentCnt := 0
			called := map[string]bool{}
			lock := sync.Mutex{}

			updateFunc := func(k string) int {
				lock.Lock()
				defer lock.Unlock()

				assert.False(t, concurrentCnt > numKeys)
				assert.False(t, called[k])
				called[k] = true
				concurrentCnt++
				lock.Unlock()
				time.Sleep(10 * time.Millisecond)
				lock.Lock()
				concurrentCnt--

				return 123
			}

			ctx := context.Background()
			sc := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.SyncRead = true
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})

			var wg sync.WaitGroup

			for i := 0; i < 1000; i++ {
				wg.Add(1)

				k := strconv.Itoa(i % numKeys)

				go func() {
					defer wg.Done()

					key := "key" + k
					val, err := sc.Get(ctx, key, func(ctx context.Context) (int, error) {
						return updateFunc(key), nil
					})

					assert.NoError(t, err)
					assert.Equal(t, 123, val)
				}()
			}

			wg.Wait()
		})
	}
}

func TestFailoverBy_Get_ErrorConcurrency(t *testing.T) {
	for _, be := range backendsBy[int]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			numKeys := 10
			concurrentCnt := 0
			called := map[string]bool{}
			lock := sync.Mutex{}

			failFunc := func(k string) error {
				lock.Lock()
				defer lock.Unlock()

				assert.False(t, concurrentCnt > numKeys)
				assert.False(t, called[k])
				called[k] = true
				concurrentCnt++
				lock.Unlock()
				time.Sleep(10 * time.Millisecond)
				lock.Lock()
				concurrentCnt--

				return assert.AnError
			}

			ctx := context.Background()
			sc := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})

			var wg sync.WaitGroup

			for i := 0; i < 1000; i++ {
				wg.Add(1)

				k := strconv.Itoa(i % numKeys)

				go func() {
					defer wg.Done()

					key := "key" + k
					val, err := sc.Get(ctx, key, func(ctx context.Context) (int, error) {
						return 0, failFunc(key)
					})

					assert.Error(t, err)
					assert.Equal(t, 0, val)
				}()
			}

			wg.Wait()
		})
	}
}

func TestFailoverBy_Get_FailedUpdateTTL(t *testing.T) {
	for _, be := range backendsBy[string]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			cnt := 0
			buildFunc := func(ctx context.Context) (string, error) {
				cnt++

				return "a discarded value", assert.AnError
			}

			ctx := context.Background()

			c := cache.NewFailoverBy[string, string](func(cfg *cache.FailoverConfigBy[string, string]) {
				cfg.FailedUpdateTTL = -1
				cfg.Backend = be.(cache.ReadWriterBy[string, string])
			})

			val, err := c.Get(ctx, "key", buildFunc)
			assert.Error(t, err)
			assert.Empty(t, val)
			assert.Equal(t, 1, cnt)

			val, err = c.Get(ctx, "key", buildFunc)
			assert.Error(t, err)
			assert.Empty(t, val)
			assert.Equal(t, 2, cnt)

			c = cache.NewFailoverBy[string, string]()

			cnt = 0
			val, err = c.Get(ctx, "key", buildFunc)
			assert.Error(t, err)
			assert.Empty(t, val)
			assert.Equal(t, 1, cnt)

			val, err = c.Get(ctx, "key", buildFunc)
			assert.Error(t, err)
			assert.Empty(t, val)
			assert.Equal(t, 1, cnt)
		})
	}
}

func TestFailoverBy_Get_BackgroundUpdate(t *testing.T) {
	for _, be := range backendsBy[string](func(config *cache.Config) {
		config.TimeToLive = time.Millisecond
		config.ExpirationJitter = -1
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			cnt := int64(0)
			ctx := context.Background()
			c := cache.NewFailoverBy[string, string](func(cfg *cache.FailoverConfigBy[string, string]) {
				cfg.Backend = be.(cache.ReadWriterBy[string, string])
				cfg.SyncRead = true
				cfg.Logger = ctxd.NoOpLogger{}
			})

			val, err := c.Get(ctx, "key", func(ctx context.Context) (string, error) {
				atomic.AddInt64(&cnt, 1)

				return "first value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "first value", val)
			assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))
			time.Sleep(time.Millisecond)

			val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), "key", func(ctx context.Context) (string, error) {
				atomic.AddInt64(&cnt, 1)

				time.Sleep(time.Millisecond)

				return "second value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "first value", val)
			assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))

			time.Sleep(10 * time.Millisecond)

			val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), "key", func(ctx context.Context) (string, error) {
				assert.Fail(t, "should not be here")

				return "not relevant", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "second value", val)
			assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
		})
	}
}

func TestFailoverBy_Get_BackgroundUpdateMaxExpiration(t *testing.T) {
	for _, be := range backendsBy[string](func(config *cache.Config) {
		config.TimeToLive = time.Millisecond
		config.ExpirationJitter = -1
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			cnt := int64(0)
			ctx := context.Background()
			c := cache.NewFailoverBy[string, string](func(cfg *cache.FailoverConfigBy[string, string]) {
				cfg.Logger = ctxd.NoOpLogger{}
				cfg.MaxStaleness = time.Nanosecond
				cfg.Backend = be.(cache.ReadWriterBy[string, string])
			})

			val, err := c.Get(ctx, "key", func(ctx context.Context) (string, error) {
				atomic.AddInt64(&cnt, 1)

				return "first value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "first value", val)
			assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))
			time.Sleep(time.Millisecond)

			val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), "key", func(ctx context.Context) (string, error) {
				atomic.AddInt64(&cnt, 1)

				time.Sleep(time.Millisecond)

				return "second value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "second value", val)
			assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
		})
	}
}

func TestFailoverBy_Get_SyncUpdate(t *testing.T) {
	for _, be := range backendsBy[string](func(config *cache.Config) {
		config.TimeToLive = time.Millisecond
		config.ExpirationJitter = -1
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			cnt := 0
			ctx := context.Background()
			c := cache.NewFailoverBy[string, string](func(cfg *cache.FailoverConfigBy[string, string]) {
				cfg.SyncUpdate = true
				cfg.Backend = be.(cache.ReadWriterBy[string, string])
				cfg.Logger = ctxd.NoOpLogger{}
			})

			val, err := c.Get(ctx, "key", func(ctx context.Context) (string, error) {
				cnt++

				return "first value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, "first value", val)
			assert.Equal(t, 1, cnt)
			time.Sleep(5 * time.Millisecond)

			val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), "key", func(ctx context.Context) (string, error) {
				cnt++

				return "second value", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, 2, cnt)
			assert.Equal(t, "second value", val)
		})
	}
}

func TestFailoverBy_Get_staleBlock(t *testing.T) {
	st := &stats.TrackerMock{}

	for _, be := range backendsBy[int](func(config *cache.Config) {
		config.Stats = st
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			*st = stats.TrackerMock{}
			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.SyncUpdate = true
				cfg.Stats = st
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})
			ctx := context.Background()

			keysCount := 200
			pipeline := make(chan struct{}, keysCount)
			k := "oneone"

			v, err := c.Get(cache.WithTTL(ctx, time.Nanosecond, false), k, func(ctx context.Context) (int, error) {
				return 123, nil
			})
			assert.Equal(t, 123, v)
			assert.NoError(t, err)

			time.Sleep(5 * time.Microsecond)

			blockUpdate := make(chan struct{})

			for i := 0; i < keysCount; i++ {
				pipeline <- struct{}{}

				go func() {
					defer func() {
						<-pipeline
					}()

					v, err := c.Get(cache.WithTTL(ctx, time.Minute, false), k, func(ctx context.Context) (int, error) {
						<-blockUpdate

						return 123, nil
					})
					assert.Equal(t, 123, v)
					assert.NoError(t, err)
				}()
			}

			for i := 0; i < cap(pipeline)-1; i++ {
				select {
				case pipeline <- struct{}{}:
				case <-time.After(10 * time.Second):
					assert.Failf(t, "failed to finish cache reads in reasonable time", "goroutines stuck: %d of %d", keysCount-i, keysCount)

					return
				}
			}

			close(blockUpdate)

			select {
			case pipeline <- struct{}{}:
			case <-time.After(10 * time.Second):
				assert.Fail(t, "failed to finish last cache read in reasonable time")
			}
		})
	}
}

func TestFailoverBy_Get_staleValue(t *testing.T) {
	st := &stats.TrackerMock{}

	for _, be := range backendsBy[int](func(config *cache.Config) {
		config.Stats = st
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			*st = stats.TrackerMock{}

			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.SyncUpdate = true
				cfg.Stats = st
				cfg.SyncRead = true
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})
			ctx := context.Background()

			concurrencyLimit := 200
			pipeline := make(chan struct{}, concurrencyLimit)

			n := 2000
			chunk := 100
			keysCount := n / chunk

			for i := 0; i < keysCount; i++ {
				k := "oneone" + strconv.Itoa(i)

				v, err := c.Get(cache.WithTTL(ctx, -1, false), k, func(ctx context.Context) (int, error) {
					return 123, nil
				})

				assert.Equal(t, 123, v)
				assert.NoError(t, err)
			}

			for i := 0; i < n; i++ {
				k := "oneone" + strconv.Itoa(i/chunk)

				pipeline <- struct{}{}

				go func() {
					defer func() {
						<-pipeline
					}()

					v, err := c.Get(cache.WithTTL(ctx, time.Hour, false), k, func(ctx context.Context) (int, error) {
						time.Sleep(10 * time.Millisecond)

						return 123, nil
					})
					assert.Equal(t, 123, v)
					assert.NoError(t, err)
				}()
			}

			for i := 0; i < cap(pipeline); i++ {
				pipeline <- struct{}{}
			}

			assert.Equal(t, keysCount, st.Int(cache.MetricRefreshed))
		})
	}
}

func TestFailoverBy_Get_updateErr(t *testing.T) {
	for _, be := range backendsBy[int]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			ctx := context.Background()
			key := "some-key"

			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})

			v, err := c.Get(ctx, key, func(ctx context.Context) (int, error) {
				return 123, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, 123, v)

			ex, ok := be.(expireAllBy)
			if !ok {
				return
			}

			ex.ExpireAll(ctx)

			v, err = c.Get(ctx, key, func(ctx context.Context) (int, error) {
				return 0, assert.AnError
			})
			assert.NoError(t, err)
			assert.Equal(t, 123, v)
		})
	}
}

func TestFailoverBy_Get_misses(t *testing.T) {
	st := &stats.TrackerMock{}

	for _, be := range backendsBy[int](func(config *cache.Config) {
		config.Stats = st
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			*st = stats.TrackerMock{}

			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.Stats = st
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})
			ctx := context.Background()

			n := 1000
			concurrencyLimit := 200
			pipeline := make(chan struct{}, concurrencyLimit)

			for i := 0; i < n; i++ {
				pipeline <- struct{}{}

				k := "oneone" + strconv.Itoa(i)

				go func() {
					defer func() {
						<-pipeline
					}()

					v, err := c.Get(cache.WithTTL(ctx, time.Nanosecond, false), k, func(ctx context.Context) (int, error) {
						return 123, nil
					})
					assert.Equal(t, 123, v)
					assert.NoError(t, err)
				}()
			}

			for i := 0; i < cap(pipeline); i++ {
				pipeline <- struct{}{}
			}

			assert.Equal(t, n, st.Int(cache.MetricWrite))
			assert.Equal(t, n, st.Int(cache.MetricBuild))
			assert.Equal(t, 2*n, st.Int(cache.MetricMiss))
		})
	}
}

func TestFailoverBy_Get_alwaysFail(t *testing.T) {
	for _, be := range backendsBy[int](func(config *cache.Config) {
		config.TimeToLive = time.Minute
	}) {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})
			ctx := context.Background()

			n := 200
			concurrencyLimit := 10
			pipeline := make(chan struct{}, concurrencyLimit)

			for i := 0; i < n; i++ {
				pipeline <- struct{}{}

				k := "key"

				go func() {
					defer func() {
						<-pipeline
					}()

					v, err := c.Get(cache.WithTTL(ctx, time.Minute, false), k, func(ctx context.Context) (int, error) {
						return 0, assert.AnError
					})
					assert.Equal(t, 0, v)
					require.EqualError(t, err, assert.AnError.Error())
				}()
			}

			for i := 0; i < cap(pipeline); i++ {
				pipeline <- struct{}{}
			}
		})
	}
}

func TestFailoverBy_Get_mutability(t *testing.T) {
	for _, be := range backendsBy[int]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			s := &stats.TrackerMock{}
			c := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.SyncUpdate = true
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
				cfg.Stats = s
				cfg.ObserveMutability = true
			})
			ctx := context.TODO()

			_, err := c.Get(ctx, "key", func(ctx context.Context) (int, error) {
				return 123, nil
			})
			assert.NoError(t, err)

			ex, ok := be.(expireAllBy)
			if !ok {
				return
			}

			ex.ExpireAll(ctx)
			time.Sleep(time.Millisecond)
			assert.Equal(t, 1, s.Int(cache.MetricBuild))
			assert.Equal(t, 0, s.Int(cache.MetricChanged))

			_, err = c.Get(ctx, "key", func(ctx context.Context) (int, error) {
				return 123, nil
			})
			assert.NoError(t, err)
			ex.ExpireAll(ctx)

			time.Sleep(time.Millisecond)
			assert.Equal(t, 2, s.Int(cache.MetricBuild))
			assert.Equal(t, 0, s.Int(cache.MetricChanged))

			_, err = c.Get(ctx, "key", func(ctx context.Context) (int, error) {
				return 321, nil
			})
			assert.NoError(t, err)

			time.Sleep(time.Millisecond)
			assert.Equal(t, 3, s.Int(cache.MetricBuild))
			assert.Equal(t, 1, s.Int(cache.MetricChanged))
		})
	}
}

func TestFailoverBy_Get_keyLock(t *testing.T) {
	for _, be := range backendsBy[int]() {
		be := be

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			concurrentCnt := 0

			ctx := context.Background()
			sc := cache.NewFailoverBy[string, int](func(cfg *cache.FailoverConfigBy[string, int]) {
				cfg.SyncUpdate = true
				cfg.Backend = be.(cache.ReadWriterBy[string, int])
			})

			updateFunc := func(_ string) int {
				concurrentCnt++
				assert.Equal(t, 1, concurrentCnt)
				time.Sleep(time.Millisecond)

				concurrentCnt--

				return 123
			}

			var wg sync.WaitGroup

			key := "key"

			for i := 0; i < 1000; i++ {
				wg.Add(1)

				go func() {
					defer wg.Done()

					val, err := sc.Get(cache.WithTTL(ctx, -1, false), key, func(ctx context.Context) (int, error) {
						return updateFunc(key), nil
					})

					assert.NoError(t, err)
					assert.Equal(t, 123, val)
				}()
			}

			wg.Wait()
		})
	}
}
