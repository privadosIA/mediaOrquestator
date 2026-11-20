// Package httpclient makes the HTTP clients the program uses to talk to other programs (Ollama, the song services, a
// browser, the dataset hosts): they always have a timeout, so a server that never answers cannot hold a request
// forever, and a helper reads a body only up to a limit.
package httpclient

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Timeouts of the two kinds of call. A model answering without streaming can take minutes, so Long is generous; a probe
// that asks "are you there?" gets Short.
const (
	LongTimeout  = 30 * time.Minute
	ShortTimeout = 30 * time.Second
)

// New returns a client whose calls end after timeout whatever the server does, and whose connection and TLS handshake
// are attempted for at most 10 seconds. A call that carries a context with an earlier deadline ends there.
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 4,
		},
	}
}

// Long is for calls that may take minutes (a model writing an answer); Short is for probes and small reads.
var (
	Long  = New(LongTimeout)
	Short = New(ShortTimeout)
)

// ReadAllLimited reads r to its end and fails when it holds more than max bytes, instead of filling the memory.
func ReadAllLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("the answer is larger than %d MB", max>>20)
	}
	return b, nil
}
