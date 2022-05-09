//go:build go1.18
// +build go1.18

package cache_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/bool64/cache"
)

func ExampleHTTPTransfer_Export_generic() {
	ctx := context.Background()
	cacheExporter := cache.HTTPTransfer{}

	mux := http.NewServeMux()
	mux.Handle("/debug/transfer-cache", cacheExporter.Export())
	srv := httptest.NewServer(mux)

	defer srv.Close()

	// Cached entities must have exported fields to be transferable with reflection-based "encoding/gob".
	type GenericCachedEntity struct {
		Value string
	}

	// With generic cache it is not strictly necessary to register in gob, the transfer will still work.
	// However, registering to gob also calculates structural hash to avoid cache transfer when the
	// structures have changed.
	cache.GobRegister(GenericCachedEntity{})

	// Exported cache(s).
	someEntityCache := cache.NewShardedMapOf[GenericCachedEntity]()
	_ = someEntityCache.Write(ctx, []byte("key1"), GenericCachedEntity{Value: "foo"})

	// Registry of caches.
	cacheExporter.AddCache("some-entity", someEntityCache.WalkDumpRestorer())

	// Importing cache(s).
	someEntityCacheOfNewInstance := cache.NewShardedMapOf[GenericCachedEntity]()

	// Caches registry.
	cacheImporter := cache.HTTPTransfer{}
	cacheImporter.AddCache("some-entity", someEntityCacheOfNewInstance.WalkDumpRestorer())

	_ = cacheImporter.Import(ctx, srv.URL+"/debug/transfer-cache")

	val, _ := someEntityCacheOfNewInstance.Read(ctx, []byte("key1"))

	fmt.Println(val.Value)
	// Output: foo
}
