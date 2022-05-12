package cache

import (
	"context"
	"encoding"
	"fmt"
)

// BinaryEncoding defines binary transmission protocol.
type BinaryEncoding interface {
	Encode(ctx context.Context, value interface{}) ([]byte, error)
	Decode(ctx context.Context, buf []byte) (interface{}, error)
}

// BinaryUnmarshaler implements BinaryEncoding.
type BinaryUnmarshaler func(data []byte) (encoding.BinaryMarshaler, error)

// Encode encodes value to bytes.
func (f BinaryUnmarshaler) Encode(_ context.Context, value interface{}) ([]byte, error) {
	b, ok := value.(encoding.BinaryMarshaler)

	if !ok {
		return nil, fmt.Errorf("%w: %T received, BinaryMarshaler expected", ErrUnexpectedType, value)
	}

	return b.MarshalBinary()
}

// Decode decodes value from bytes.
func (f BinaryUnmarshaler) Decode(_ context.Context, buf []byte) (interface{}, error) {
	return f(buf)
}
