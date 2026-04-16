package filecache_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/bool64/cache"
	"github.com/bool64/cache/blob"
	"github.com/bool64/cache/filecache"
)

func ExampleFailover() {
	ctx := context.Background()

	dir, err := os.MkdirTemp("", "cache-filecache-example-*")
	if err != nil {
		log.Fatal(err)
	}

	defer os.RemoveAll(dir)

	st, err := filecache.NewStorage(dir)
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = st.Close()
	}()

	f := cache.NewFailoverOf[blob.Entry](func(cfg *cache.FailoverConfigOf[blob.Entry]) {
		cfg.Backend = st
	})

	entry, err := f.Get(ctx, []byte("photo:1"), func(ctx context.Context) (blob.Entry, error) {
		return blob.FromReader(bytes.NewBufferString("hello blob"), blob.Meta{
			Name: "photo.txt",
		}), nil
	})
	if err != nil {
		panic(err)
	}

	rc, err := entry.Open()
	if err != nil {
		panic(err)
	}
	defer rc.Close()

	b, err := io.ReadAll(rc)
	if err != nil {
		panic(err)
	}

	fmt.Println(entry.Meta().Name, string(b))

	// Output:
	// photo.txt hello blob
}
