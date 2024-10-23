module benchmark

go 1.18

replace github.com/bool64/cache => ../

require (
	github.com/VictoriaMetrics/fastcache v1.10.0
	github.com/allegro/bigcache/v3 v3.0.2
	github.com/bool64/cache v0.0.0-00010101000000-000000000000
	github.com/coocood/freecache v1.2.1
	github.com/dgraph-io/ristretto v0.1.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/puzpuzpuz/xsync v1.2.1
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/sys v0.15.0 // indirect
)
