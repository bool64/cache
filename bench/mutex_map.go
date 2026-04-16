package bench

import (
	"sync"
	"testing"
)

// MutexMap is a benchmark runner.
type MutexMap struct {
	mu          sync.Mutex
	c           map[string]SmallCachedValue
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r *MutexMap) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := make(map[string]SmallCachedValue, cardinality)
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c[keys[i]] = MakeCachedValue(i)
	}

	return &MutexMap{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "mutexMap-base"
}

// Run iterates over the cache.
func (r *MutexMap) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality
		k := r.keys[i]

		w++
		if w == writeEvery {
			w = 0

			k = r.writeKeys[i]

			r.mu.Lock()
			r.c[k] = MakeCachedValue(i)
			r.mu.Unlock()

			r.mu.Lock()
			delete(r.c, k)
			r.mu.Unlock()

			continue
		}

		r.mu.Lock()
		v, found := r.c[k]
		r.mu.Unlock()

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}

// RWMutexMap is a benchmark runner.
type RWMutexMap struct {
	mu          sync.RWMutex
	c           map[string]SmallCachedValue
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r *RWMutexMap) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := make(map[string]SmallCachedValue, cardinality)
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c[keys[i]] = MakeCachedValue(i)
	}

	return &RWMutexMap{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "rwMutexMap-base"
}

// Run iterates over the cache.
func (r *RWMutexMap) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality
		k := r.keys[i]

		w++
		if w == writeEvery {
			w = 0

			k = r.writeKeys[i]

			r.mu.Lock()
			r.c[k] = MakeCachedValue(i)
			r.mu.Unlock()

			r.mu.Lock()
			delete(r.c, k)
			r.mu.Unlock()

			continue
		}

		r.mu.RLock()
		v, found := r.c[k]
		r.mu.RUnlock()

		if !found || v.I != i {
			b.Fatalf("found: %v, val: %v", found, v)
		}
	}
}
