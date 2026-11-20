package runner

import (
	"strings"
	"sync"
)

// TailBuffer is an io.Writer that keeps only the last Max bytes written to it, for the error messages of a program
// that may write a lot.
type TailBuffer struct {
	mu  sync.Mutex
	Max int
	b   []byte
}

// Write keeps the last Max bytes of everything written so far; it never fails.
func (t *TailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.Max {
		t.b = append([]byte(nil), t.b[len(t.b)-t.Max:]...)
	}
	return len(p), nil
}

// Bytes returns a copy of what is kept.
func (t *TailBuffer) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.b...)
}

// String returns what is kept, without the white space at its ends.
func (t *TailBuffer) String() string { return strings.TrimSpace(string(t.Bytes())) }
