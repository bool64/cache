module benchmark

go 1.21

replace github.com/bool64/cache => ../

require (
	github.com/VictoriaMetrics/fastcache v1.12.2
	github.com/allegro/bigcache/v3 v3.1.0
	github.com/bool64/cache v0.0.0-00010101000000-000000000000
	github.com/coocood/freecache v1.2.4
	github.com/dgraph-io/ristretto v0.1.1
	github.com/maypok86/otter v1.1.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/puzpuzpuz/xsync v1.5.2
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/dolthub/swiss v0.2.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/golang/glog v1.2.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
