package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

// Body limits of the routes that receive text a person can make long: a scenes.yaml, or the dataset an import maps.
const (
	MaxConfigBody = 1 << 20  // a scenes.yaml sent as JSON
	MaxImportBody = 16 << 20 // the text of a dataset to import
)

// DecodeJSONLimited reads the JSON body of r into v, refusing a body larger than limit bytes. It answers the request
// itself when it fails (413 naming the limit, or 400 for a body that is not JSON) and reports whether v is filled.
func DecodeJSONLimited(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	RaiseBodyLimit(w, r, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteError(w, http.StatusRequestEntityTooLarge, CodeInvalid, fmt.Sprintf("the request body is larger than %d MB", max(limit>>20, 1)))
			return false
		}
		WriteError(w, http.StatusBadRequest, CodeInvalid, fmt.Sprintf("invalid JSON body: %v", err))
		return false
	}
	return true
}

// DefaultBodyLimit is the size of a request body no handler asked to raise: a JSON message, not an upload.
const DefaultBodyLimit = 1 << 20

// limitedBody is the body the server gives every handler: limited to DefaultBodyLimit, and remembering the
// original so a handler that takes uploads can ask for more. It records that the limit was hit.
type limitedBody struct {
	io.ReadCloser
	orig io.ReadCloser
	hit  *atomic.Bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		b.hit.Store(true)
	}
	return n, err
}

// overflowWriter turns the 400 a handler answers when its body could not be read because of the limit into the 413 it
// means, so the many handlers that decode their own JSON need no change.
type overflowWriter struct {
	http.ResponseWriter
	hit *atomic.Bool
}

func (w *overflowWriter) WriteHeader(status int) {
	if status == http.StatusBadRequest && w.hit.Load() {
		status = http.StatusRequestEntityTooLarge
	}
	w.ResponseWriter.WriteHeader(status)
}

// Flush keeps event streams working through the wrapper.
func (w *overflowWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *overflowWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// LimitBody wraps h so its request body is limited to DefaultBodyLimit; a body over it fails to read with a
// *http.MaxBytesError, and the 400 a handler answers for it becomes 413. A handler that takes a bigger body (an
// upload, a package, a long text) calls RaiseBodyLimit first.
func LimitBody(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody {
			h(w, r)
			return
		}
		hit := &atomic.Bool{}
		r.Body = &limitedBody{ReadCloser: http.MaxBytesReader(w, r.Body, DefaultBodyLimit), orig: r.Body, hit: hit}
		h(&overflowWriter{ResponseWriter: w, hit: hit}, r)
	}
}

// RaiseBodyLimit sets the limit of the request body to limit bytes, in place of the default one. A body over it fails to
// read with a *http.MaxBytesError; nothing beyond limit is kept in memory or in the temporary files of a multipart form.
func RaiseBodyLimit(w http.ResponseWriter, r *http.Request, limit int64) {
	if lb, ok := r.Body.(*limitedBody); ok {
		r.Body = &limitedBody{ReadCloser: http.MaxBytesReader(w, lb.orig, limit), orig: lb.orig, hit: lb.hit}
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
}
