package bench

import (
	"strconv"
	"sync"
	"testing"
)

// MutexMap is a benchmark runner.
type MutexMap struct {
	mu          sync.Mutex
	c           map[string]SmallCachedValue
	cardinality int
}

// Make initializes benchmark runner.
func (r *MutexMap) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := make(map[string]SmallCachedValue, cardinality)
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c[string(buf)] = MakeCachedValue(i)
	}

	return &MutexMap{
		c:           c,
		cardinality: cardinality,
	}, "mutexMap-base"
}

// Run iterates over the cache.
func (r *MutexMap) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		k := string(buf)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			k := string(buf)

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
}

// Make initializes benchmark runner.
func (r *RWMutexMap) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := make(map[string]SmallCachedValue, cardinality)
	buf := make([]byte, 0)

	for i := 0; i < cardinality; i++ {
		i := i

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		c[string(buf)] = MakeCachedValue(i)
	}

	return &RWMutexMap{
		c:           c,
		cardinality: cardinality,
	}, "rwMutexMap-base"
}

// Run iterates over the cache.
func (r *RWMutexMap) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	buf := make([]byte, 0, 10)
	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality

		buf = append(buf[:0], []byte(KeyPrefix)...)
		buf = append(buf, []byte(strconv.Itoa(i))...)

		k := string(buf)

		w++
		if w == writeEvery {
			w = 0

			buf = append(buf, 'n') // Insert new key.

			k := string(buf)

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
