package cache_test

import (
	"bytes"
	"context"
	"errors"
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
)

func TestFailover_Get_ValueConcurrency(t *testing.T) {
	numKeys := 10 // making 10 distinct keys
	concurrentCnt := 0
	called := map[string]bool{}
	lock := sync.Mutex{}

	// some serious and slow processing may happen here
	updateFunc := func(k string) interface{} {
		lock.Lock()
		defer lock.Unlock()

		assert.False(t, concurrentCnt > numKeys, "must not have running concurrency more than number of keys")
		assert.False(t, called[k], "must not be called twice, second read should be from cache")
		called[k] = true
		concurrentCnt++
		lock.Unlock()
		time.Sleep(10 * time.Millisecond)
		lock.Lock()
		concurrentCnt--

		return 123
	}

	ctx := context.Background()
	sc := cache.NewFailover(cache.FailoverConfig{
		SyncRead: true,
	}.Use)

	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)

		k := strconv.Itoa(i % numKeys)

		go func() {
			defer wg.Done()

			key := "key" + k
			val, err := sc.Get(ctx, []byte(key), func(ctx context.Context) (interface{}, error) {
				return updateFunc(key), nil
			})

			assert.NoError(t, err)
			assert.Equal(t, 123, val)
		}()
	}

	wg.Wait()
}

func TestFailover_Get_ErrorConcurrency(t *testing.T) {
	numKeys := 10 // making 10 distinct keys
	concurrentCnt := 0
	called := map[interface{}]bool{}
	lock := sync.Mutex{}

	// some serious and slow processing may happen here
	failFunc := func(k string) error {
		lock.Lock()
		defer lock.Unlock()

		assert.False(t, concurrentCnt > numKeys, "must not have running concurrency more than number of keys")
		assert.False(t, called[k], "must not be called twice, second read should be from cache")
		called[k] = true
		concurrentCnt++
		lock.Unlock()
		time.Sleep(10 * time.Millisecond)
		lock.Lock()
		concurrentCnt--

		return errors.New("failed")
	}

	ctx := context.Background()
	sc := cache.NewFailover()

	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)

		k := strconv.Itoa(i % numKeys)

		go func() {
			defer wg.Done()

			key := "key" + k
			val, err := sc.Get(ctx, []byte(key), func(ctx context.Context) (interface{}, error) {
				return nil, failFunc(key)
			})

			assert.Error(t, err)
			assert.Nil(t, val)
		}()
	}

	wg.Wait()
}

func TestFailover_Get_FailedUpdateTTL(t *testing.T) {
	cnt := 0
	buildFunc := func(ctx context.Context) (i interface{}, e error) {
		cnt++

		return "a discarded value", errors.New("failed")
	}

	ctx := context.Background()

	c := cache.NewFailover(
		cache.FailoverConfig{
			FailedUpdateTTL: -1,
		}.Use)

	val, err := c.Get(ctx, []byte("key"), buildFunc)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Equal(t, 1, cnt)

	val, err = c.Get(ctx, []byte("key"), buildFunc)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Equal(t, 2, cnt)

	c = cache.NewFailover()

	cnt = 0
	val, err = c.Get(ctx, []byte("key"), buildFunc)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Equal(t, 1, cnt)

	val, err = c.Get(ctx, []byte("key"), buildFunc)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Equal(t, 1, cnt)

	val, err = c.Get(ctx, []byte("key"), buildFunc)
	assert.Error(t, err)
	assert.Nil(t, val)
	assert.Equal(t, 1, cnt)
}

func TestFailover_Get_BackgroundUpdate(t *testing.T) {
	cnt := int64(0)
	ctx := context.Background()
	logger := ctxd.NoOpLogger{}
	c := cache.NewFailover(
		cache.FailoverConfig{
			Logger: logger,
			BackendConfig: cache.Config{
				TimeToLive: time.Millisecond,
			},
			SyncRead: true,
		}.Use,
	)

	val, err := c.Get(ctx, []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)

		return "first value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "first value", val)
	assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))
	time.Sleep(time.Millisecond)

	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		time.Sleep(time.Millisecond)
		atomic.AddInt64(&cnt, 1)

		return "second value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "first value", val) // Stale value immediately returned, background update.
	assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))

	time.Sleep(10 * time.Millisecond)

	// Should not call buildFunc here.
	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)
		assert.Fail(t, "should not be here")

		return "not relevant", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "second value", val) // Updated value.
	assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
}

func TestFailover_Get_BackgroundUpdateMaxExpiration(t *testing.T) {
	cnt := int64(0)
	ctx := context.Background()
	logger := ctxd.NoOpLogger{}
	c := cache.NewFailover(
		cache.FailoverConfig{
			Logger:       logger,
			MaxStaleness: time.Nanosecond,
			BackendConfig: cache.Config{
				TimeToLive: time.Millisecond,
			},
		}.Use,
	)

	val, err := c.Get(ctx, []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)

		return "first value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "first value", val)
	assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))
	time.Sleep(time.Millisecond)

	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		time.Sleep(time.Millisecond)
		atomic.AddInt64(&cnt, 1)

		return "second value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "second value", val) // Stale value not returned due to MaxStaleness, background update.
	assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))

	time.Sleep(10 * time.Millisecond)

	// Should not call buildFunc here.
	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)
		assert.Fail(t, "should not be here")

		return "not relevant", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "second value", val) // Updated value.
	assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
}

func TestFailover_Get_SyncUpdate(t *testing.T) {
	cnt := int64(0)
	ctx := context.Background()
	c := cache.NewFailover(
		cache.FailoverConfig{
			SyncUpdate: true,
			Logger:     ctxd.NoOpLogger{},
			BackendConfig: cache.Config{
				TimeToLive: time.Millisecond,
			},
		}.Use,
	)

	val, err := c.Get(ctx, []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)

		return "first value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "first value", val)
	assert.Equal(t, int64(1), atomic.LoadInt64(&cnt))
	time.Sleep(5 * time.Millisecond)

	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)

		return "second value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
	assert.Equal(t, "second value", val) // Updated value returned.

	time.Sleep(10 * time.Millisecond)

	// Should not call buildFunc here.
	val, err = c.Get(cache.WithTTL(ctx, time.Minute, false), []byte("key"), func(ctx context.Context) (i interface{}, e error) {
		atomic.AddInt64(&cnt, 1)
		assert.Fail(t, "should not be here")

		return "not relevant", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "second value", val) // Updated value.
	assert.Equal(t, int64(2), atomic.LoadInt64(&cnt))
}

func TestFailover_Get_updateErr(t *testing.T) {
	ctx := context.Background()
	key := "some-key"

	mc := cache.NewShardedMap()

	c := cache.NewFailover(cache.FailoverConfig{
		Backend: mc,
	}.Use)

	v, err := c.Get(ctx, []byte(key), func(ctx context.Context) (interface{}, error) {
		return 123, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 123, v)

	mc.ExpireAll(ctx)

	v, err = c.Get(ctx, []byte(key), func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("failed")
	})

	// in case of update error stale value is served
	assert.NoError(t, err)
	assert.Equal(t, 123, v)

	v, err = c.Get(ctx, []byte(key), func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("failed")
	})

	// in case of update error stale value is served
	assert.NoError(t, err)
	assert.Equal(t, 123, v)
}

func TestFailover_Get_mutability(t *testing.T) {
	s := &stats.TrackerMock{}
	mc := cache.NewShardedMap()
	c := cache.NewFailover(cache.FailoverConfig{
		SyncUpdate:        true,
		Backend:           mc,
		Stats:             s,
		ObserveMutability: true,
	}.Use)
	ctx := context.TODO()

	// First value served.
	_, err := c.Get(ctx, []byte("key"), func(ctx context.Context) (interface{}, error) {
		return 123, nil
	})
	assert.NoError(t, err)
	mc.ExpireAll(ctx)

	time.Sleep(time.Millisecond)
	assert.Equal(t, 1, s.Int(cache.MetricBuild))
	assert.Equal(t, 0, s.Int(cache.MetricChanged))

	// Second value served equal to first, no change.
	_, err = c.Get(ctx, []byte("key"), func(ctx context.Context) (interface{}, error) {
		return 123, nil
	})
	assert.NoError(t, err)
	mc.ExpireAll(ctx)

	time.Sleep(time.Millisecond)
	assert.Equal(t, 2, s.Int(cache.MetricBuild))
	assert.Equal(t, 0, s.Int(cache.MetricChanged))

	// Third value served not equal to second, change counted.
	_, err = c.Get(ctx, []byte("key"), func(ctx context.Context) (interface{}, error) {
		return 321, nil
	})
	assert.NoError(t, err)
	mc.ExpireAll(ctx)

	time.Sleep(time.Millisecond)
	assert.Equal(t, 3, s.Int(cache.MetricBuild))
	assert.Equal(t, 1, s.Int(cache.MetricChanged))
}

func TestFailover_Get_keyLock(t *testing.T) {
	concurrentCnt := int64(0)

	ctx := context.Background()
	sc := cache.NewFailover(cache.FailoverConfig{SyncUpdate: true}.Use)

	// some serious and slow processing may happen here
	updateFunc := func(_ []byte) interface{} {
		cnt := atomic.AddInt64(&concurrentCnt, 1)
		assert.Equal(t, int64(1), cnt, "must not have running concurrency")
		time.Sleep(time.Millisecond)
		atomic.AddInt64(&concurrentCnt, -1)

		return 123
	}

	var wg sync.WaitGroup

	key := []byte("key")

	for i := 0; i < 1000; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			val, err := sc.Get(cache.WithTTL(ctx, cache.SkipWriteTTL, false), key, func(ctx context.Context) (interface{}, error) {
				return updateFunc(key), nil
			})

			assert.NoError(t, err)
			assert.Equal(t, 123, val)
		}()
	}

	wg.Wait()
}

// seq is the logging sequence.
var seq int64

// sequencer adds sequential number to every log record.
var sequencer = ctxd.DeferredJSON(func() interface{} {
	return atomic.AddInt64(&seq, 1)
})

func TestFailover_Get_lowCardinalityKey(t *testing.T) {
	st := &stats.TrackerMock{}
	l := ctxd.LoggerMock{}
	c := cache.NewFailover(cache.FailoverConfig{
		Stats:    st,
		SyncRead: true,
		Logger:   &l,
	}.Use)

	concurrencyLimit := 500
	pipeline := make(chan struct{}, concurrencyLimit)

	n := 10000
	chunk := 100
	ctxs := make(map[string]context.Context, 1000)
	bufs := make(map[string]*bytes.Buffer, 1000)

	for i := 0; i <= n/chunk; i++ {
		k := "oneone" + strconv.Itoa(i)

		_, ok := ctxs[k]
		if !ok {
			buf := &bytes.Buffer{}
			ctx := ctxd.AddFields(context.Background(), "keySeq", atomic.LoadInt64(&seq), "logSeq", sequencer)
			ctx = ctxd.WithLogWriter(ctx, buf)
			ctxs[k] = ctx
			bufs[k] = buf
		}

		atomic.AddInt64(&seq, 1)
	}

	for i := 0; i < n; i++ {
		pipeline <- struct{}{}

		// Distinct key for every chunk.
		k := "oneone" + strconv.Itoa(i/chunk)

		go func() {
			defer func() {
				<-pipeline
			}()

			atomic.AddInt64(&seq, 1)
			ctx := ctxd.AddFields(ctxs[k], "tx", atomic.LoadInt64(&seq))
			v, err := c.Get(cache.WithTTL(ctx, time.Minute, false), []byte(k), func(ctx context.Context) (interface{}, error) {
				time.Sleep(10 * time.Microsecond)

				return 123, nil
			})
			assert.Equal(t, 123, v)
			assert.NoError(t, err)
		}()
	}

	// Waiting for goroutines to finish.
	for i := 0; i < cap(pipeline); i++ {
		pipeline <- struct{}{}
	}

	// Every distinct key has single build and write.
	expectedWrites := n / chunk
	assert.Equal(t, expectedWrites, st.Int(cache.MetricWrite), "total writes")

	if !assert.Equal(t, expectedWrites, st.Int(cache.MetricBuild), "total builds") {
		for _, buf := range bufs {
			if bytes.Count(buf.Bytes(), []byte("wrote")) > 1 {
				assert.Fail(t, "unexpected multiple cache writes", buf.String())

				break
			}
		}
	}

	// Written value is returned without hitting cache.
	assert.Equal(t, n-expectedWrites, st.Int(cache.MetricHit))
}

func TestFailover_Get_staleBlock(t *testing.T) {
	st := &stats.TrackerMock{}
	c := cache.NewFailover(cache.FailoverConfig{
		SyncUpdate: true,
		Stats:      st,
	}.Use)
	ctx := context.Background()

	keysCount := 500
	pipeline := make(chan struct{}, keysCount)

	// Distinct key for every chunk.
	k := "oneone"

	v, err := c.Get(cache.WithTTL(ctx, time.Nanosecond, false), []byte(k), func(ctx context.Context) (interface{}, error) {
		return 123, nil
	})
	assert.Equal(t, 123, v)
	assert.NoError(t, err)

	assert.Equal(t, 1, st.Int(cache.MetricWrite))
	assert.Equal(t, 1, st.Int(cache.MetricBuild))

	// No hits for initial population.
	assert.Equal(t, 0, st.Int(cache.MetricHit))

	// Expiring values.
	time.Sleep(5 * time.Microsecond)

	blockUpdate := make(chan struct{})

	// Hitting stale values.
	for i := 0; i < keysCount; i++ {
		pipeline <- struct{}{}

		go func() {
			defer func() {
				<-pipeline
			}()

			v, err := c.Get(cache.WithTTL(ctx, time.Minute, false), []byte(k), func(ctx context.Context) (interface{}, error) {
				<-blockUpdate

				return 123, nil
			})
			assert.Equal(t, 123, v)
			assert.NoError(t, err)
		}()
	}

	// Waiting for goroutines to finish.
	// One goroutine left to finish update.
	for i := 0; i < cap(pipeline)-1; i++ {
		select {
		case pipeline <- struct{}{}:
		case <-time.After(10 * time.Second):
			assert.Failf(t,
				"failed to finish cache reads in reasonable time, probably due to blocked stale refresh",
				"goroutines stuck: %d of %d", keysCount-i, keysCount)

			return
		}
	}
	close(blockUpdate)

	// Waiting for last goroutine.
	select {
	case pipeline <- struct{}{}:
	case <-time.After(10 * time.Second):
		assert.Fail(t, "failed to finish last cache read in reasonable time")
	}

	assert.Equal(t, 3, st.Int(cache.MetricWrite), "total writes")
	assert.Equal(t, 2, st.Int(cache.MetricBuild), "total builds")

	// Every request hits cached or expired value.
	assert.Equal(t, keysCount, st.Int(cache.MetricHit)+st.Int(cache.MetricExpired))
}

func TestFailover_Get_staleValue(t *testing.T) {
	st := &stats.TrackerMock{}
	c := cache.NewFailover(cache.FailoverConfig{
		SyncUpdate: true,
		Stats:      st,
		SyncRead:   true,
	}.Use)
	ctx := context.Background()

	concurrencyLimit := 500
	pipeline := make(chan struct{}, concurrencyLimit)

	n := 10000
	chunk := 100
	keysCount := n / chunk

	// Populating initial values.
	for i := 0; i < keysCount; i++ {
		// Distinct key for every chunk.
		k := "oneone" + strconv.Itoa(i)

		// Storing expired values.
		v, err := c.Get(cache.WithTTL(ctx, cache.SkipWriteTTL, false), []byte(k), func(ctx context.Context) (interface{}, error) {
			return 123, nil
		})

		assert.Equal(t, 123, v)
		assert.NoError(t, err)
	}

	// Every distinct key has single build and write.
	expectedWrites := keysCount
	assert.Equal(t, expectedWrites, st.Int(cache.MetricWrite))
	assert.Equal(t, expectedWrites, st.Int(cache.MetricBuild))

	// No hits for initial population.
	assert.Equal(t, 0, st.Int(cache.MetricHit))

	// Hitting stale values.
	for i := 0; i < n; i++ {
		pipeline <- struct{}{}

		// Distinct key for every chunk.
		k := "oneone" + strconv.Itoa(i/chunk)

		go func() {
			defer func() {
				<-pipeline
			}()

			v, err := c.Get(cache.WithTTL(ctx, time.Hour, false), []byte(k), func(ctx context.Context) (interface{}, error) {
				time.Sleep(10 * time.Millisecond)

				return 123, nil
			})
			assert.Equal(t, 123, v)
			assert.NoError(t, err)
		}()
	}

	// Waiting for goroutines to finish.
	for i := 0; i < cap(pipeline); i++ {
		pipeline <- struct{}{}
	}

	// Every distinct key has three writes:
	//  initial population write,
	//  stale value refresh,
	//  non-stale write.
	assert.Equalf(t, 3*keysCount, st.Int(cache.MetricWrite),
		"write expectation failed, stats: %+v", st.Values())

	// Every distinct key has two builds:
	//  initial population,
	//  stale update.
	assert.Equalf(t, 2*keysCount, st.Int(cache.MetricBuild),
		"build expectation failed, stats: %+v", st.Values())

	// Every key's stale value is refreshed once.
	assert.Equalf(t, keysCount, st.Int(cache.MetricRefreshed),
		"staleServed expectation failed, stats: %+v", st.Values())

	// All requests hit cached or expired value.
	assert.Equalf(t, n, st.Int(cache.MetricHit)+st.Int(cache.MetricExpired),
		"hit+expired expectation failed, stats: %+v", st.Values())
}

func TestFailover_Get_misses(t *testing.T) {
	st := &stats.TrackerMock{}
	c := cache.NewFailover(cache.FailoverConfig{
		Stats: st,
	}.Use)
	ctx := context.Background()

	n := 10000

	concurrencyLimit := 1000
	pipeline := make(chan struct{}, concurrencyLimit)

	// Populating n values.
	for i := 0; i < n; i++ {
		pipeline <- struct{}{}

		k := "oneone" + strconv.Itoa(i)

		go func() {
			defer func() {
				<-pipeline
			}()

			v, err := c.Get(cache.WithTTL(ctx, time.Nanosecond, false), []byte(k), func(ctx context.Context) (interface{}, error) {
				return 123, nil
			})
			assert.Equal(t, 123, v)
			assert.NoError(t, err)
		}()
	}

	// Waiting for goroutines to finish.
	for i := 0; i < cap(pipeline); i++ {
		pipeline <- struct{}{}
	}

	// Every distinct key has single build and write.
	expectedWrites := n
	assert.Equal(t, expectedWrites, st.Int(cache.MetricWrite))
	assert.Equal(t, expectedWrites, st.Int(cache.MetricBuild))

	// Every new value has 2 misses:
	//   initial miss in backend,
	//   missing cached build error.
	assert.Equal(t, 2*n, st.Int(cache.MetricMiss))
}

func TestFailover_Get_alwaysFail(t *testing.T) {
	c := cache.NewFailover(cache.FailoverConfig{
		BackendConfig: cache.Config{TimeToLive: time.Minute},
	}.Use)
	ctx := context.Background()

	n := 500

	concurrencyLimit := 10
	pipeline := make(chan struct{}, concurrencyLimit)

	// Populating n values.
	for i := 0; i < n; i++ {
		pipeline <- struct{}{}

		k := "key"

		go func() {
			defer func() {
				<-pipeline
			}()

			v, err := c.Get(cache.WithTTL(ctx, time.Minute, false), []byte(k), func(ctx context.Context) (interface{}, error) {
				return nil, errors.New("failed")
			})
			assert.Equal(t, nil, v)
			require.EqualError(t, err, "failed")
		}()
	}

	// Waiting for goroutines to finish.
	for i := 0; i < cap(pipeline); i++ {
		pipeline <- struct{}{}
	}
}
