package cache_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/bool64/cache"
)

func ExampleHTTPTransfer_Export() {
	ctx := context.Background()
	cacheExporter := cache.HTTPTransfer{}

	mux := http.NewServeMux()
	mux.Handle("/debug/transfer-cache", cacheExporter.Export())
	srv := httptest.NewServer(mux)

	defer srv.Close()

	// Cached entities must have exported fields to be transferable with reflection-based "encoding/gob".
	type SomeCachedEntity struct {
		Value string
	}

	// Cached entity types must be registered to gob, this can be done in init functions of cache facades.
	cache.GobRegister(SomeCachedEntity{})

	// Exported cache(s).
	someEntityCache := cache.NewShardedMap()
	_ = someEntityCache.Write(ctx, []byte("key1"), SomeCachedEntity{Value: "foo"})

	// Registry of caches.
	cacheExporter.AddCache("some-entity", someEntityCache)

	// Importing cache(s).
	someEntityCacheOfNewInstance := cache.NewShardedMap()

	// Caches registry.
	cacheImporter := cache.HTTPTransfer{}
	cacheImporter.AddCache("some-entity", someEntityCacheOfNewInstance)

	_ = cacheImporter.Import(ctx, srv.URL+"/debug/transfer-cache")

	val, _ := someEntityCacheOfNewInstance.Read(ctx, []byte("key1"))

	fmt.Println(val.(SomeCachedEntity).Value)
	// Output: foo
}
