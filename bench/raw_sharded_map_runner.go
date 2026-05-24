package bench

import "testing"

// RawShardedStringMapBaseline is a benchmark runner for a thin sharded string map.
type RawShardedStringMapBaseline struct {
	c           *RawShardedMap[string, SmallCachedValue]
	cardinality int
	keys        []string
	writeKeys   []string
}

// Make initializes benchmark runner.
func (r RawShardedStringMapBaseline) Make(b *testing.B, cardinality int) (Runner, string) {
	b.Helper()

	c := NewRawShardedMap[string, SmallCachedValue](fnv32a)
	keys, writeKeys := makeStringKeys(cardinality)

	for i := 0; i < cardinality; i++ {
		c.Store(keys[i], MakeCachedValue(i))
	}

	return RawShardedStringMapBaseline{
		c:           c,
		cardinality: cardinality,
		keys:        keys,
		writeKeys:   writeKeys,
	}, "rawShardedMap-base"
}

// Run iterates over the cache.
func (r RawShardedStringMapBaseline) Run(b *testing.B, cnt int, writeEvery int) {
	b.Helper()

	w := 0

	for i := 0; i < cnt; i++ {
		i := (i ^ 12345) % r.cardinality
		key := r.keys[i]

		w++
		if w == writeEvery {
			w = 0
			key = r.writeKeys[i]

			r.c.Store(key, MakeCachedValue(i))
			r.c.Delete(key)

			continue
		}

		v, ok := r.c.Load(key)
		if !ok || v.I != i {
			b.Fatalf("found: %v, val: %v", ok, v)
		}
	}
}
