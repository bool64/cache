package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/bool64/ctxd"
)

// HTTPTransfer exports and imports cache entries via http.
type HTTPTransfer struct {
	Logger    ctxd.Logger
	Transport http.RoundTripper

	caches map[string]WalkDumpRestorer
}

// AddCache registers cache into exporter.
func (t *HTTPTransfer) AddCache(name string, c WalkDumpRestorer) {
	if t.caches == nil {
		t.caches = make(map[string]WalkDumpRestorer)
	}

	t.caches[name] = c
}

// ExportJSONL creates http handler to export cache entries as JSON lines.
func (t *HTTPTransfer) ExportJSONL() http.Handler {
	logger := t.Logger

	if logger == nil {
		logger = ctxd.NoOpLogger{}
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")

		if name != "" {
			_, ok := t.caches[name]
			if !ok {
				http.Error(rw, "cache not found for "+name, http.StatusNotFound)

				return
			}
		}
		start := time.Now()
		w := writerCnt{w: rw}

		var (
			n   int
			err error
		)

		enc := json.NewEncoder(&w)
		enc.SetEscapeHTML(false)

		line := make(map[string]interface{}, 4)
		for cn, c := range t.caches {
			if name != "" && cn != name {
				continue
			}

			cn := cn
			n, err = c.Walk(func(entry Entry) error {
				line["name"] = cn
				line["key"] = string(entry.Key())
				line["expireAt"] = entry.ExpireAt()
				line["value"] = entry.Value()

				return enc.Encode(line)
			})

			ctx := ctxd.AddFields(r.Context(),
				"processed", n,
				"elapsed", time.Since(start).String(),
				"bytes", atomic.LoadInt64(&w.n),
				"speed", fmt.Sprintf("%f MB/s", float64(atomic.LoadInt64(&w.n))/(1024*1024*time.Since(start).Seconds())),
				"name", name,
			)

			if err != nil {
				logger.Error(ctx, "failed to dump cache", "error", err)

				return
			}

			logger.Important(ctx, "cache dump finished")
		}
	})
}

// Export creates http handler to export cache entries in encoding/gob format.
func (t *HTTPTransfer) Export() http.Handler {
	logger := t.Logger

	if logger == nil {
		logger = ctxd.NoOpLogger{}
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(rw, "missing name query parameter", http.StatusBadRequest)

			return
		}

		c, ok := t.caches[name]
		if !ok {
			http.Error(rw, "cache not found for "+name, http.StatusNotFound)

			return
		}

		start := time.Now()
		w := writerCnt{w: rw}

		var (
			n   int
			err error
		)

		typesHash := r.URL.Query().Get("typesHash")
		if typesHash == "" {
			http.Error(rw, "missing typesHash query parameter", http.StatusBadRequest)

			return
		}

		if typesHash != strconv.FormatUint(GobTypesHash(), 10) {
			logger.Warn(r.Context(), "cache dump failed: typesHash mismatch, incompatible cache")
			http.Error(rw, "typesHash mismatch, incompatible cache", http.StatusBadRequest)

			return
		}

		rw.Header().Set("Content-Type", "application/octet-stream")
		n, err = c.Dump(&w)

		ctx := ctxd.AddFields(r.Context(),
			"processed", n,
			"elapsed", time.Since(start).String(),
			"bytes", atomic.LoadInt64(&w.n),
			"speed", fmt.Sprintf("%f MB/s", float64(atomic.LoadInt64(&w.n))/(1024*1024*time.Since(start).Seconds())),
			"name", name,
		)
		if err != nil {
			logger.Error(ctx, "failed to dump cache", "error", err)

			return
		}
		logger.Important(ctx, "cache dump finished")
	})
}

// Import loads cache entries exported at exportURL.
func (t *HTTPTransfer) Import(ctx context.Context, exportURL string) error {
	u, err := url.Parse(exportURL)
	if err != nil {
		return err
	}

	typesHash := strconv.FormatUint(GobTypesHash(), 10)
	logger := t.Logger

	if logger == nil {
		logger = ctxd.NoOpLogger{}
	}

	for name, c := range t.caches {
		q := u.Query()

		q.Set("name", name)
		q.Set("typesHash", typesHash)
		u.RawQuery = q.Encode()

		ctx := ctxd.AddFields(ctx, "name", name, "url", u.String())

		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			logger.Warn(ctx, "failed to build request", "error", err)

			continue
		}

		tr := t.Transport
		if tr == nil {
			tr = http.DefaultTransport
		}

		resp, err := tr.RoundTrip(req)
		if err != nil {
			logger.Warn(ctx, "failed to send cache dump request", "error", err)

			continue
		}

		if resp.StatusCode == http.StatusOK {
			start := time.Now()
			r := readerCnt{r: resp.Body}
			n, err := c.Restore(&r)
			ctx = ctxd.AddFields(ctx,
				"processed", n,
				"elapsed", time.Since(start).String(),
				"speed", fmt.Sprintf("%f MB/s", float64(atomic.LoadInt64(&r.n))/(1024*1024*time.Since(start).Seconds())),
				"bytes", atomic.LoadInt64(&r.n),
			)

			if err != nil {
				logger.Warn(ctx, "failed to restore cache dump", "error", err)
			} else {
				logger.Important(ctx, "cache restored")
			}
		} else {
			res, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				logger.Warn(ctx, "failed to read response body", "error", err)
			} else {
				logger.Warn(ctx, "cache restore failed", "resp", string(res))
			}
		}

		_, err = io.Copy(ioutil.Discard, resp.Body)
		if err != nil {
			logger.Warn(ctx, "failed to flush response body", "error", err)
		}

		err = resp.Body.Close()
		if err != nil {
			logger.Warn(ctx, "failed to close response body", "error", err)
		}
	}

	return nil
}

type writerCnt struct {
	w io.Writer
	n int64
}

func (w *writerCnt) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	atomic.AddInt64(&w.n, int64(n))

	return n, err
}

type readerCnt struct {
	r io.Reader
	n int64
}

func (r *readerCnt) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	atomic.AddInt64(&r.n, int64(n))

	return n, err
}
