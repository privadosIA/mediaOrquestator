package api

import (
	"context"
	"sync"
)

// EventLog receives the pipeline emitter's JSON lines (it is the emitter's
// io.Writer in jsonMode), keeps each line verbatim, persists it through
// sink, and wakes subscribers. The byte slice appended is exactly what the
// CLI would have written to stdout, minus the trailing newline.
type EventLog struct {
	mu     sync.Mutex
	cond   *sync.Cond
	lines  [][]byte
	closed bool
	sink   func(seq int, line []byte)
}

// NewEventLog returns an empty log; sink, when not nil, is called with every line as it is appended (the runs persist them there).
func NewEventLog(sink func(seq int, line []byte)) *EventLog {
	l := &EventLog{sink: sink}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// Write implements io.Writer for the events.Emitter, which writes exactly
// one line (JSON object + "\n") per call.
func (l *EventLog) Write(p []byte) (int, error) {
	line := make([]byte, len(p))
	copy(line, p)
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	l.mu.Lock()
	seq := len(l.lines)
	l.lines = append(l.lines, line)
	sink := l.sink
	l.cond.Broadcast()
	l.mu.Unlock()
	if sink != nil {
		sink(seq, line)
	}
	return len(p), nil
}

// Close marks the stream complete and wakes every waiter.
func (l *EventLog) Close() {
	l.mu.Lock()
	l.closed = true
	l.cond.Broadcast()
	l.mu.Unlock()
}

// Next blocks until line i exists, the log is closed, or ctx is done; it
// returns the line (nil when none) and whether the stream is over for
// this reader. Callers must have arranged for a broadcast on ctx
// cancellation (WatchCancel) or they may block past it.
func (l *EventLog) Next(ctx context.Context, i int) ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(l.lines) <= i && !l.closed && ctx.Err() == nil {
		l.cond.Wait()
	}
	if ctx.Err() != nil {
		return nil, true
	}
	if i < len(l.lines) {
		return l.lines[i], false
	}
	return nil, true
}

// WatchCancel wakes Next() waiters when ctx is cancelled.
func (l *EventLog) WatchCancel(ctx context.Context) {
	go func() {
		<-ctx.Done()
		l.mu.Lock()
		l.cond.Broadcast()
		l.mu.Unlock()
	}()
}
