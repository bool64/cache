//go:build go1.18
// +build go1.18

package cache

import (
	"fmt"
	"reflect"

	"github.com/cespare/xxhash/v2"
)

func resolveShardFunc[K comparable](cfg ConfigBy[K]) func(K) uint64 {
	if cfg.ShardFunc != nil {
		return cfg.ShardFunc
	}

	var zero K

	switch any(zero).(type) {
	case string:
		return func(key K) uint64 {
			return xxhash.Sum64String(any(key).(string))
		}
	case bool:
		return func(key K) uint64 {
			if any(key).(bool) {
				return 1
			}

			return 0
		}
	case int:
		return func(key K) uint64 { return mixInt64(int64(any(key).(int))) }
	case int8:
		return func(key K) uint64 { return mixInt64(int64(any(key).(int8))) }
	case int16:
		return func(key K) uint64 { return mixInt64(int64(any(key).(int16))) }
	case int32:
		return func(key K) uint64 { return mixInt64(int64(any(key).(int32))) }
	case int64:
		return func(key K) uint64 { return mixInt64(any(key).(int64)) }
	case uint:
		return func(key K) uint64 { return mixUint64(uint64(any(key).(uint))) }
	case uint8:
		return func(key K) uint64 { return mixUint64(uint64(any(key).(uint8))) }
	case uint16:
		return func(key K) uint64 { return mixUint64(uint64(any(key).(uint16))) }
	case uint32:
		return func(key K) uint64 { return mixUint64(uint64(any(key).(uint32))) }
	case uint64:
		return func(key K) uint64 { return mixUint64(any(key).(uint64)) }
	case uintptr:
		return func(key K) uint64 { return mixUint64(uint64(any(key).(uintptr))) }
	}

	t := reflect.TypeOf(zero)
	if t == nil {
		panic("cache: no default key sharder for K=<nil>; configure ShardFunc in keyed config/options")
	}

	switch t.Kind() {
	case reflect.String:
		return func(key K) uint64 {
			return xxhash.Sum64String(reflect.ValueOf(key).Convert(reflect.TypeOf("")).String())
		}
	case reflect.Bool:
		return func(key K) uint64 {
			if reflect.ValueOf(key).Bool() {
				return 1
			}

			return 0
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(key K) uint64 {
			return mixInt64(reflect.ValueOf(key).Int())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(key K) uint64 {
			return mixUint64(reflect.ValueOf(key).Uint())
		}
	default:
		panic(fmt.Sprintf("cache: no default key sharder for K=%s; configure ShardFunc in keyed config/options", t.String()))
	}
}

func mixUint64(v uint64) uint64 {
	v += 0x9e3779b97f4a7c15
	v = (v ^ (v >> 30)) * 0xbf58476d1ce4e5b9
	v = (v ^ (v >> 27)) * 0x94d049bb133111eb

	return v ^ (v >> 31)
}

func mixInt64(v int64) uint64 {
	if v >= 0 {
		return mixUint64(uint64(v) * 2)
	}

	//nolint:gosec // Value is known to be non-negative after the branch and transformation.
	return mixUint64(uint64(-(v+1))*2 + 1)
}
