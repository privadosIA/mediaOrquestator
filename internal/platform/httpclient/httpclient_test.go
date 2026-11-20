package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew_AServerThatNeverAnswersTimesOut(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	start := time.Now()
	_, err := New(200 * time.Millisecond).Get(srv.URL)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("want a timeout error within a few seconds, got %v after %v", err, time.Since(start))
	}
}

func TestReadAllLimited(t *testing.T) {
	if b, err := ReadAllLimited(strings.NewReader("hello"), 5); err != nil || string(b) != "hello" {
		t.Fatalf("at the limit: %q, %v", b, err)
	}
	if _, err := ReadAllLimited(strings.NewReader(strings.Repeat("x", 3<<20)), 2<<20); err == nil {
		t.Fatal("a body over the limit must fail")
	}
}

func TestSharedClientsHaveTimeouts(t *testing.T) {
	if Long.Timeout <= 0 || Short.Timeout <= 0 {
		t.Fatal("the shared clients must have timeouts")
	}
}
