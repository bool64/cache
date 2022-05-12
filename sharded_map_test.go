package cache_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/bool64/cache"
	"github.com/bool64/ctxd"
	"github.com/bool64/stats"
	"github.com/stretchr/testify/assert"
	"github.com/swaggest/assertjson"
)

func TestNewShardedMap(t *testing.T) {
	logger := ctxd.LoggerMock{}
	st := stats.TrackerMock{}

	func() {
		c := cache.NewShardedMap(func(config *cache.Config) {
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
		assert.Nil(t, v)
		assert.Equal(t, "bar", err.(cache.ErrWithExpiredItem).Value())

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
