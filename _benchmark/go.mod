module benchmark

go 1.24.0

replace github.com/bool64/cache => ../

require (
	github.com/VictoriaMetrics/fastcache v1.13.0
	github.com/allegro/bigcache/v3 v3.1.0
	github.com/bool64/cache v0.0.0-00010101000000-000000000000
	github.com/coocood/freecache v1.2.4
	github.com/dgraph-io/ristretto v0.2.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/puzpuzpuz/xsync/v4 v4.2.0
	golang.org/x/sync v0.17.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/sys v0.37.0 // indirect
)
