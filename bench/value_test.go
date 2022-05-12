package bench_test

import (
	"testing"

	"github.com/bool64/cache/bench"
	"github.com/stretchr/testify/assert"
)

func TestSmallCachedValue(t *testing.T) {
	s1 := bench.SmallCachedValue{B: true, I: -123, S: bench.LongString}

	b, err := s1.MarshalBinary()
	assert.NoError(t, err)

	s2 := bench.SmallCachedValue{}
	assert.NoError(t, s2.UnmarshalBinary(b))
	assert.Equal(t, s1.B, s2.B)
	assert.Equal(t, s1.I, s2.I)
	assert.Equal(t, s1.S, s2.S)
}
