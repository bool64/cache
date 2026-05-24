package bench

import (
	"context"
	"sync"

	"github.com/bool64/cache"
)

const rawShards = 128

type rawShardedMapShard[K comparable, V any] struct {
	sync.RWMutex
	data map[K]V
}

// RawShardedMap is a thin generic sharded primitive over map[K]V.
type RawShardedMap[K comparable, V any] struct {
	shardFn func(K) uint32
	shards  [rawShards]rawShardedMapShard[K, V]
}

// NewRawShardedMap creates a sharded map primitive for benchmarks.
func NewRawShardedMap[K comparable, V any](shardFn func(K) uint32) *RawShardedMap[K, V] {
	m := &RawShardedMap[K, V]{shardFn: shardFn}

	for i := range m.shards {
		m.shards[i].data = make(map[K]V)
	}

	return m
}

func (m *RawShardedMap[K, V]) shard(key K) *rawShardedMapShard[K, V] {
	return &m.shards[m.shardFn(key)%rawShards]
}

// Load gets value by key.
func (m *RawShardedMap[K, V]) Load(key K) (V, bool) {
	s := m.shard(key)
	s.RLock()
	v, ok := s.data[key]
	s.RUnlock()

	return v, ok
}

// Store saves value by key.
func (m *RawShardedMap[K, V]) Store(key K, value V) {
	s := m.shard(key)
	s.Lock()
	s.data[key] = value
	s.Unlock()
}

// Delete removes value by key.
func (m *RawShardedMap[K, V]) Delete(key K) {
	s := m.shard(key)
	s.Lock()
	delete(s.data, key)
	s.Unlock()
}

// ThinShardedMap is a bench-local thin cache.ReadWriter wrapper over RawShardedMap.
type ThinShardedMap struct {
	primitive *RawShardedMap[string, SmallCachedValue]
}

// NewThinShardedMap creates a thin cache.ReadWriter wrapper for benchmarks.
func NewThinShardedMap() *ThinShardedMap {
	return &ThinShardedMap{primitive: NewRawShardedMap[string, SmallCachedValue](fnv32a)}
}

// Read gets cached value.
func (m *ThinShardedMap) Read(ctx context.Context, key []byte) (interface{}, error) {
	if cache.SkipRead(ctx) {
		return nil, cache.ErrNotFound
	}

	v, ok := m.primitive.Load(string(key))
	if !ok {
		return nil, cache.ErrNotFound
	}

	return v, nil
}

// Write stores cached value.
func (m *ThinShardedMap) Write(_ context.Context, key []byte, value interface{}) error {
	m.primitive.Store(string(key), value.(SmallCachedValue))

	return nil
}

// Delete removes cached value.
func (m *ThinShardedMap) Delete(_ context.Context, key []byte) error {
	m.primitive.Delete(string(key))

	return nil
}

func fnv32a(key string) uint32 {
	var h uint32 = 2166136261

	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}

	return h
}
