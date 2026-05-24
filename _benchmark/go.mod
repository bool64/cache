module benchmark

go 1.25.0

replace github.com/bool64/cache => ../

require (
	github.com/VictoriaMetrics/fastcache v1.13.3
	github.com/allegro/bigcache/v3 v3.1.0
	github.com/bool64/cache v0.0.0-00010101000000-000000000000
	github.com/coocood/freecache v1.2.7
	github.com/dgraph-io/ristretto v0.2.0
	github.com/jeremiah-masters/dlht v0.0.0-20260507233412-e7a3bb1532f6
	github.com/maypok86/otter/v2 v2.3.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/puzpuzpuz/xsync/v4 v4.5.0
	golang.org/x/sync v0.20.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/sys v0.45.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
