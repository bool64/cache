package blob_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bool64/cache/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromReader(t *testing.T) {
	entry := blob.FromReader(bytes.NewBufferString("hello"), blob.Meta{Name: "greeting.txt"})

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)

	assert.Equal(t, "hello", string(b))
	assert.Equal(t, "greeting.txt", entry.Meta().Name)
}

func TestFromHTTPResponse(t *testing.T) {
	reqURL, err := url.Parse("https://example.com/images/photo.jpg")
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode:    http.StatusCreated,
		ContentLength: 5,
		Header: http.Header{
			"Last-Modified": []string{"Mon, 02 Jan 2006 15:04:05 GMT"},
			"Content-Type":  []string{"image/jpeg"},
		},
		Body: io.NopCloser(bytes.NewBufferString("image")),
		Request: &http.Request{
			URL: reqURL,
		},
	}

	entry := blob.FromHTTPResponse(resp)

	meta := entry.Meta()
	assert.Equal(t, "photo.jpg", meta.Name)
	assert.Equal(t, int64(5), meta.Size)
	assert.Equal(t, time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC), meta.ModTime)

	extra, ok := meta.Extra.(blob.HTTPExtra)
	require.True(t, ok)
	assert.Equal(t, http.StatusCreated, extra.StatusCode)
	assert.Equal(t, "image/jpeg", extra.Header.Get("Content-Type"))

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "image", string(b))

	_, err = entry.Open()
	assert.ErrorIs(t, err, io.ErrClosedPipe)
}
