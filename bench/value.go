package bench

import (
	"encoding"
	"encoding/binary"
	"strconv"

	"github.com/bool64/cache"
)

// Sample values.
const (
	LongString = "looooooooooooooooooooooooooongstring"
	KeyPrefix  = "thekey"
)

// SmallCachedValue represents a small value for a cached item.
type SmallCachedValue struct {
	B bool
	S string
	I int
}

var (
	_ encoding.BinaryUnmarshaler = &SmallCachedValue{}
	_ encoding.BinaryMarshaler   = &SmallCachedValue{}
)

// MarshalBinary encodes SmallCachedValue to bytes.
func (s SmallCachedValue) MarshalBinary() (data []byte, err error) {
	b := make([]byte, 9, 9+len(s.S))
	binary.LittleEndian.PutUint64(b, uint64(s.I))

	if s.B {
		b[8] = 1
	}

	b = append(b, []byte(s.S)...)

	return b, nil
}

// UnmarshalBinary loads value from bytes.
func (s *SmallCachedValue) UnmarshalBinary(data []byte) error {
	s.I = int(binary.LittleEndian.Uint64(data))
	s.B = data[8] == 1
	s.S = string(data[9:])

	return nil
}

// MakeCachedValue creates a SmallCachedValue.
func MakeCachedValue(i int) SmallCachedValue {
	return SmallCachedValue{
		I: i,
		S: LongString + strconv.Itoa(i),
		B: true,
	}
}

func init() {
	cache.GobRegister(SmallCachedValue{})
}
