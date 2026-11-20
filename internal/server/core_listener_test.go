package server

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestListener_HasTimeoutsAndNoWholeRequestTimeout(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadHeaderTimeout > 30*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 1s..30s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 || srv.IdleTimeout > 5*time.Minute {
		t.Errorf("IdleTimeout = %v, want at most 5 min", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes <= 0 || srv.MaxHeaderBytes > 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want at most 1 MiB", srv.MaxHeaderBytes)
	}
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Errorf("Read/WriteTimeout = %v/%v: uploads and event streams must not be cut", srv.ReadTimeout, srv.WriteTimeout)
	}
}

func TestListener_ASlowHeaderClientIsClosed(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	srv.ReadHeaderTimeout = 200 * time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /api/health HTTP/1.1\r\nHost: x\r\nX-Slow: "))
	// The headers never finish: the server must close the connection soon after the timeout.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("the server did not close a connection that never finished its headers: %v", err)
	}
}
