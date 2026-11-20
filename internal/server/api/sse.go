package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEWriter frames messages for one text/event-stream response.
type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// NewSSEWriter starts a server-sent-events response on w; false when w cannot flush, in which case nothing was written.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &SSEWriter{w: w, f: f}, true
}

// Event sends one pipeline event line as a default-type SSE message. The
// data bytes are exactly the JSON line the emitter produced.
func (s *SSEWriter) Event(line []byte) {
	fmt.Fprintf(s.w, "data: %s\n\n", line)
	s.f.Flush()
}

// Notify sends a server-originated message under its own SSE event type.
func (s *SSEWriter) Notify(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, data)
	s.f.Flush()
}

// Named sends one SSE message under its own event type with a ready-made JSON payload.
func (sw *SSEWriter) Named(eventType string, data []byte) {
	fmt.Fprintf(sw.w, "event: %s\ndata: %s\n\n", eventType, data)
	sw.f.Flush()
}
