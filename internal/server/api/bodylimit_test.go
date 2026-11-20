package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalid, "invalid JSON body: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, v)
}

func jsonBody(n int) io.Reader {
	return strings.NewReader(`{"x":"` + strings.Repeat("a", n) + `"}`)
}

func TestLimitBody_AJSONBodyOverTheDefaultIs413(t *testing.T) {
	h := LimitBody(jsonHandler)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/", jsonBody(2<<20)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413 (%s)", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/", jsonBody(100)))
	if rec.Code != http.StatusOK {
		t.Fatalf("a small body: status %d, want 200", rec.Code)
	}
}

func TestLimitBody_AMalformedBodyStaysA400(t *testing.T) {
	rec := httptest.NewRecorder()
	LimitBody(jsonHandler)(rec, httptest.NewRequest("POST", "/", strings.NewReader("{nope")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestRaiseBodyLimit_AnUploadMayBeBiggerButNotWithoutEnd(t *testing.T) {
	h := LimitBody(func(w http.ResponseWriter, r *http.Request) {
		RaiseBodyLimit(w, r, 3<<20)
		jsonHandler(w, r)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/", jsonBody(2<<20)))
	if rec.Code != http.StatusOK {
		t.Fatalf("2 MiB under a 3 MiB limit: status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/", jsonBody(4<<20)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("4 MiB over a 3 MiB limit: status %d, want 413", rec.Code)
	}
}

func TestRaiseBodyLimit_AnExtraMultipartPartDoesNotFillTheTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	h := LimitBody(func(w http.ResponseWriter, r *http.Request) {
		RaiseBodyLimit(w, r, 2<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			WriteError(w, http.StatusBadRequest, CodeInvalid, "invalid multipart body: "+err.Error())
			return
		}
		defer r.MultipartForm.RemoveAll()
		w.WriteHeader(http.StatusOK)
	})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("extra", "extra.bin")
	part.Write(bytes.Repeat([]byte{1}, 6<<20))
	mw.Close()
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", rec.Code)
	}
	var left int64
	ents, _ := os.ReadDir(tmp)
	for _, e := range ents {
		if fi, err := e.Info(); err == nil {
			left += fi.Size()
		}
	}
	if left > 3<<20 {
		t.Fatalf("%d bytes left in the temp dir, want at most the limit", left)
	}
}

func TestLimitBody_StreamsStillFlush(t *testing.T) {
	flushed := false
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder(), onFlush: func() { flushed = true }}
	LimitBody(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
	})(rec, httptest.NewRequest("GET", "/", http.NoBody))
	if !flushed {
		t.Fatal("Flush must reach the real writer")
	}
	rec = &flushRecorder{ResponseRecorder: httptest.NewRecorder(), onFlush: func() { flushed = true }}
	flushed = false
	LimitBody(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
	})(rec, httptest.NewRequest("POST", "/", strings.NewReader("x")))
	if !flushed {
		t.Fatal("Flush must reach the real writer when the body is wrapped too")
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	onFlush func()
}

func (f *flushRecorder) Flush() { f.onFlush() }
