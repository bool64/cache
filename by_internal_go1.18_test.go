//go:build go1.18
// +build go1.18

package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func backendsByInternal[V any](options ...func(*Config)) []interface {
	ReadWriterBy[string, V]
	Len() int
} {
	return []interface {
		ReadWriterBy[string, V]
		Len() int
	}{
		NewShardedMapBy[string, V](func(cfg *ConfigBy[string]) {
			for _, option := range options {
				option(&cfg.Config)
			}
		}),
		NewSyncMapBy[string, V](options...),
	}
}

type evictInterfaceBy[V any] interface {
	ReadWriterBy[string, V]
	Len() int
	evictMostExpired(evictFraction float64) int
}

func TestBy_evictHeapInuse(t *testing.T) {
	for _, be := range backendsByInternal[int](Config{
		HeapInUseSoftLimit: 1,
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterfaceBy[int])
		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			ctx := context.Background()

			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(WithTTL(ctx, time.Duration(i+1)*time.Second, false), strconv.Itoa(i), i))
			}

			assert.Equal(t, 1000, m.Len())

			m.evictMostExpired(0.1)
			assert.Equal(t, 900, m.Len())

			missing := 0
			present := 0

			for i := 0; i < 1000; i++ {
				_, err := m.Read(context.Background(), strconv.Itoa(i))
				if errors.Is(err, ErrNotFound) {
					missing++

					continue
				}

				assert.NoError(t, err)

				present++
			}

			assert.Equal(t, 100, missing)
			assert.Equal(t, 900, present)
		})
	}
}

func TestBy_evictHeapInuse_noTTL(t *testing.T) {
	for _, be := range backendsByInternal[int](Config{
		HeapInUseSoftLimit: 1,
		ExpirationJitter:   -1,
	}.Use) {
		m, ok := be.(evictInterfaceBy[int])
		require.True(t, ok)

		t.Run(fmt.Sprintf("%T", be), func(t *testing.T) {
			ctx := context.Background()

			for i := 0; i < 1000; i++ {
				require.NoError(t, m.Write(ctx, strconv.Itoa(i), i))
			}

			assert.Equal(t, 1000, m.Len())

			m.evictMostExpired(0.1)
			assert.Equal(t, 900, m.Len())

			missing := 0
			present := 0

			for i := 0; i < 1000; i++ {
				_, err := m.Read(context.Background(), strconv.Itoa(i))
				if errors.Is(err, ErrNotFound) {
					missing++

					continue
				}

				assert.NoError(t, err)

				present++
			}

			assert.Equal(t, 100, missing)
			assert.Equal(t, 900, present)
		})
	}
}
