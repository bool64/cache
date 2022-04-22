//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
	"github.com/stretchr/testify/assert"
	"github.com/swaggest/assertjson"
)

func TestNewShardedMapOf(t *testing.T) {
	logger := ctxd.LoggerMock{}
	st := stats.TrackerMock{}

	func() {
		c := cache.NewShardedMapOf[string](func(config *cache.Config) {
			config.Logger = &logger
			config.Stats = &st
			config.Name = "test"
			config.TimeToLive = time.Hour
		})

		ctx := context.Background()
		assert.NoError(t, c.Write(ctx, []byte("foo"), "bar"))
		v, err := c.Read(ctx, []byte("foo"))
		assert.NoError(t, err)
		assert.Equal(t, "bar", v)
		assert.Equal(t, 1, c.Len())

		c.ExpireAll(ctx)

		v, err = c.Read(ctx, []byte("foo"))
		assert.EqualError(t, err, cache.ErrExpired.Error())
		assert.Empty(t, v)
		assert.Equal(t, "bar", err.(cache.ErrWithExpiredItemOf[string]).Value())

		assert.NoError(t, c.Delete(ctx, []byte("foo")))
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
	// Last two entries are
	//   "closing cache janitor"
	//   "closing cache items counter goroutine"
	// they are masked with "<ignore-diff>" because they are produced with different goroutines
	// and can arrive in random order.

	assert.Equal(t, `cache_expired{name="test"} 1
cache_hit{name="test"} 1
cache_items{name="test"} 0
cache_write{name="test"} 1`, st.Metrics())
}

// shardedMapOfBaseline is a benchmark runner.
type shardedMapOfBaseline struct {
	c           *cache.ShardedMapOf[smallCachedValue]
	cardinality int
}

func (cl shardedMapOfBaseline) make(b *testing.B, cardinality int) (cacheLoader, string) {
	b.Helper()

	c := cache.NewShardedMapOf[smallCachedValue]()
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c.Store(buf, makeCachedValue(i))
	}

	return shardedMapOfBaseline{
		c:           c,
		cardinality: cardinality,
	}, "shardedMapOf-base"
}

func (cl shardedMapOfBaseline) run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0
	ctx := context.Background()

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % cl.cardinality

		buf = append(buf[:0], []byte(keyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			cl.c.Store(buf, makeCachedValue(i))

			if err := cl.c.Delete(ctx, buf); err != nil && err != cache.ErrNotFound {
				b.Fatalf("err: %v", err)
			}

			continue
		}

		v, found := cl.c.Load(buf)

		if !found || v.i != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}