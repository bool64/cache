// Package blob defines reopenable blob descriptors and helper constructors
// that can be cached with cache.ReadWriterOf or cache.FailoverOf.
package blob

import (
	"encoding/gob"
	"io"
	"net/http"
	"path"
	"sync"
	"time"
)

// Meta describes a blob.
type Meta struct {
	Name    string
	Size    int64
	ModTime time.Time
	Extra   any
}

// Entry is a reopenable descriptor of a blob.
type Entry interface {
	Meta() Meta
	Open() (io.ReadCloser, error)
}

// HTTPExtra is a conventional HTTP response payload for Meta.Extra.
type HTTPExtra struct {
	StatusCode int
	Header     http.Header
}

//nolint:gochecknoinits // Gob registration must happen at package init time for filecache dump/restore.
func init() {
	gob.Register(HTTPExtra{})
}

type transientEntry struct {
	meta Meta
	open func() (io.ReadCloser, error)
}

// Meta returns blob metadata.
func (t transientEntry) Meta() Meta {
	return t.meta
}

// Open opens the underlying reader.
func (t transientEntry) Open() (io.ReadCloser, error) {
	return t.open()
}

// FromReader creates a transient entry from an io.Reader.
func FromReader(r io.Reader, meta Meta) Entry {
	return FromReadCloser(readCloser{Reader: r}, meta)
}

// FromReadCloser creates a transient entry from an io.ReadCloser.
func FromReadCloser(rc io.ReadCloser, meta Meta) Entry {
	var (
		mu     sync.Mutex
		opened bool
	)

	return transientEntry{
		meta: meta,
		open: func() (io.ReadCloser, error) {
			mu.Lock()
			defer mu.Unlock()

			if opened {
				return nil, io.ErrClosedPipe
			}

			opened = true

			return rc, nil
		},
	}
}

// FromHTTPResponse creates a transient entry backed by an HTTP response body.
func FromHTTPResponse(resp *http.Response) Entry {
	meta := Meta{
		Name:    responseName(resp),
		Size:    responseSize(resp),
		ModTime: responseModTime(resp),
		Extra: HTTPExtra{
			StatusCode: resp.StatusCode,
			Header:     resp.Header.Clone(),
		},
	}

	return FromReadCloser(resp.Body, meta)
}

type readCloser struct {
	io.Reader
}

func (readCloser) Close() error {
	return nil
}

func responseName(resp *http.Response) string {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return ""
	}

	name := path.Base(resp.Request.URL.Path)
	if name == "." || name == "/" {
		return ""
	}

	return name
}

func responseSize(resp *http.Response) int64 {
	if resp == nil || resp.ContentLength < 0 {
		return 0
	}

	return resp.ContentLength
}

func responseModTime(resp *http.Response) time.Time {
	if resp == nil {
		return time.Time{}
	}

	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if t, err := http.ParseTime(lastModified); err == nil {
			return t
		}
	}

	return time.Time{}
}
