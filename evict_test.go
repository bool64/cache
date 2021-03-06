package cache // nolint:testpackage

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

func backends(options ...func(*Config)) []ReadWriter {
	return []ReadWriter{
		NewShardedMap(options...),
		NewSyncMap(options...),
	}
}

func TestShardedMap_evictHeapInuse(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1, // Setting heap threshold to 1B to force eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(interface {
			ReadWriter
			Len() int
			evictOldest()
			heapInUseOverflow() bool
		})

		if !ok {
			continue
		}

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			assert.Equal(t, 1000, m.Len())
			assert.True(t, m.heapInUseOverflow())

			// Keys 0-99 should be evicted by 0.1 fraction, keys 100-999 should remain.
			m.evictOldest()
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
		m, ok := be.(interface {
			ReadWriter
			Len() int
			evictOldest()
			heapInUseOverflow() bool
		})

		if !ok {
			continue
		}

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			m.heapInUseOverflow()
			assert.Equal(t, 1000, m.Len())
		})
	}
}

func TestShardedMap_evictHeapInuse_skipped(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1e10, // Setting heap threshold to big value to skip eviction.
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(interface {
			ReadWriter
			Len() int
			evictOldest()
			heapInUseOverflow() bool
		})

		if !ok {
			continue
		}

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			// expire := time.Now().Add(time.Hour)
			ctx := context.Background()

			// Filling cache with enough items.
			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), []byte(strconv.Itoa(i)), i))
			}

			m.heapInUseOverflow()
			assert.Equal(t, 1000, m.Len())
		})
	}
}

func TestShardedMap_evictHeapInuse_concurrency(t *testing.T) {
	for _, be := range backends(Config{
		HeapInUseSoftLimit: 1, // Setting heap threshold to 1B value to force eviction.
	}.Use) {
		m, ok := be.(interface {
			ReadWriter
			Len() int
			evictOldest()
			heapInUseOverflow() bool
		})

		if !ok {
			continue
		}

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			ctx := context.Background()
			wg := sync.WaitGroup{}
			wg.Add(1000)

			for i := 0; i < 1000; i++ {
				i := i

				go func() {
					defer wg.Done()

					if i%100 == 0 {
						m.heapInUseOverflow()
					}

					k := strconv.Itoa(i % 100)

					err := m.Write(ctx, []byte(k), i)
					assert.NoError(t, err)
				}()
			}

			wg.Wait()
		})
	}
}
