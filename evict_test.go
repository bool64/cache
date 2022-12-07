package cache //nolint:testpackage

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func backends(options ...func(*Config)) []interface {
	ReadWriter
	Len() int
} {
	return []interface {
		ReadWriter
		Len() int
	}{
		NewShardedMap(options...),
		NewSyncMap(options...),
	}
}

var (
	_ evictInterface = &ShardedMap{}
	_ evictInterface = &SyncMap{}
)

type evictInterface interface {
	ReadWriter
	Len() int
	evictMostExpired(evictFraction float64) int
}

func TestShardedMap_evictHeapInuse(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1, // Setting heap threshold to 1B to force eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterface)

		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			assert.Equal(t, 1000, m.Len())

			// Keys 0-99 should be evicted by 0.1 fraction, keys 100-999 should remain.
			m.evictMostExpired(0.1)
			assert.Equal(t, 900, m.Len())

			for i := 0; i < 100; i++ {
				_, err := m.Read(context.Background(), []byte(strconv.Itoa(i)))
				assert.EqualError(t, err, ErrNotFound.Error())
			}

			for i := 100; i < 1000; i++ {
				_, err := m.Read(context.Background(), []byte(strconv.Itoa(i)))
				assert.NoError(t, err)
			}
		})
	}
}

func TestShardedMap_evictHeapInuse_disabled(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 0, // Setting heap threshold to 0 to disable eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterface)

		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			assert.Equal(t, 1000, m.Len())
		})
	}
}

func TestShardedMap_evictHeapInuse_skipped(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1e10, // Setting heap threshold to big value to skip eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterface)

		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			assert.Equal(t, 1000, m.Len())
		})
	}
}

func TestShardedMap_evictHeapInuse_concurrency(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1, // Setting heap threshold to 1B value to force eviction.
	}.Use) {
		m, ok := be.(evictInterface)

		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			ctx := context.Background()
			wg := sync.WaitGroup{}
			wg.Add(1000)

			for i := 0; i < 1000; i++ {
				i := i

				go func() {
					defer wg.Done()

					k := strconv.Itoa(i % 100)

					err := m.Write(ctx, []byte(k), i)
					assert.NoError(t, err)
				}()
			}

			wg.Wait()
		})
	}
}

func TestShardedMap_evictHeapInuse_noTTL(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1, // Setting heap threshold to 1B to force eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterface)

		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(ctx, []byte(strconv.Itoa(i)), i))
			}

			assert.Equal(t, 1000, m.Len())

			// Keys 0-99 should be evicted by 0.1 fraction, keys 100-999 should remain.
			m.evictMostExpired(0.1)
			assert.Equal(t, 900, m.Len())

			for i := 0; i < 100; i++ {
				_, err := m.Read(context.Background(), []byte(strconv.Itoa(i)))
				assert.EqualError(t, err, ErrNotFound.Error())
			}

			for i := 100; i < 1000; i++ {
				_, err := m.Read(context.Background(), []byte(strconv.Itoa(i)))
				assert.NoError(t, err)
			}
		})
	}
}

func Test_LFU_eviction(t *testing.T) {
	for _, c := range backends(func(cfg *Config) {
		cfg.EvictionStrategy = EvictLeastFrequentlyUsed
		cfg.EvictFraction = 0.5
		cfg.CountSoftLimit = 100
		cfg.DeleteExpiredJobInterval = time.Millisecond
	}) {
		t.Run(fmt.Sprintf("%T", c), func(t *testing.T) {
			ctx := context.Background()

			for i := 0; i < 100; i++ {
				k := []byte(strconv.Itoa(i))
				require.NoError(t, c.Write(ctx, k, i))

				_, err := c.Read(ctx, k)
				require.NoError(t, err)

				for j := 100 - i; j > 0; j-- {
					_, err = c.Read(ctx, k)
					require.NoError(t, err)
				}
			}

			// Preparing for eviction.
			require.NoError(t, c.Write(ctx, []byte("100!"), 100))

			i := 0
			for {
				i++
				time.Sleep(10 * time.Millisecond)
				if c.Len() <= 100 {
					break
				}

				require.Less(t, i, 10, c.Len())
			}

			assert.LessOrEqual(t, c.Len(), 51)

			for i := 0; i < 51; i++ {
				k := []byte(strconv.Itoa(i))

				_, err := c.Read(ctx, k)
				require.NoError(t, err, i)
			}

			for i := 51; i < 100; i++ {
				k := []byte(strconv.Itoa(i))

				_, err := c.Read(ctx, k)
				assert.EqualError(t, err, "missing cache item", i) // Evicted.
			}
		})
	}
}

func Test_LRU_eviction(t *testing.T) {
	for _, c := range backends(func(cfg *Config) {
		cfg.EvictionStrategy = EvictLeastRecentlyUsed
		cfg.EvictFraction = 0.5
		cfg.CountSoftLimit = 100
		cfg.DeleteExpiredJobInterval = time.Millisecond
	}) {
		t.Run(fmt.Sprintf("%T", c), func(t *testing.T) {
			ctx := context.Background()

			for i := 0; i < 100; i++ {
				k := []byte(strconv.Itoa(i))
				require.NoError(t, c.Write(ctx, k, i))

				_, err := c.Read(ctx, k)
				require.NoError(t, err)

				time.Sleep(time.Microsecond)
			}

			// Preparing for eviction.
			require.NoError(t, c.Write(ctx, []byte("100!"), 100))

			i := 0
			for {
				i++
				time.Sleep(10 * time.Millisecond)
				if c.Len() <= 100 {
					break
				}

				require.Less(t, i, 10, c.Len())
			}

			assert.LessOrEqual(t, c.Len(), 51)

			for i := 0; i < 49; i++ {
				k := []byte(strconv.Itoa(i))
				_, err := c.Read(ctx, k)

				require.EqualError(t, err, "missing cache item", i) // Evicted.
			}

			for i := 49; i < 100; i++ {
				k := []byte(strconv.Itoa(i))
				_, err := c.Read(ctx, k)

				require.NoError(t, err) // Kept.
			}
		})
	}
}
