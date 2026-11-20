package workspace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseMessage is one parsed SSE frame.
type sseMessage struct {
	event string // "" = default (pipeline event)
	data  string
}

// readSSE parses frames from a live SSE body until stream-end, the
// predicate says stop, or the deadline hits.
func readSSE(t *testing.T, body io.Reader, max time.Duration) []sseMessage {
	t.Helper()
	var msgs []sseMessage
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(body)
		cur := sseMessage{}
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				cur.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if cur.data != "" || cur.event != "" {
					msgs = append(msgs, cur)
					if cur.event == "stream-end" {
						return
					}
					cur = sseMessage{}
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(max):
		t.Log("readSSE: deadline reached")
	}
	return msgs
}

func pipelineEvents(msgs []sseMessage) []string {
	var out []string
	for _, m := range msgs {
		if m.event == "" {
			out = append(out, m.data)
		}
	}
	return out
}

func TestSSE_LateSubscriberReplayThenLiveAndStreamEnd(t *testing.T) {
	s, fe := launchServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "1"})
	waitStart(t, fe) // pipeline_start already emitted

	// Late subscriber: must receive the already-emitted event first.
	resp, err := http.Get(ts.URL + "/api/runs/" + id + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		fe.chanFor(id) <- true // emits pipeline_end and finishes
	}()
	msgs := readSSE(t, resp.Body, 3*time.Second)

	evs := pipelineEvents(msgs)
	if len(evs) != 2 {
		t.Fatalf("expected 2 pipeline events (start, end), got %d: %v", len(evs), evs)
	}
	var first, last map[string]any
	if err := json.Unmarshal([]byte(evs[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(evs[len(evs)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if first["type"] != "pipeline_start" || last["type"] != "pipeline_end" {
		t.Errorf("event order wrong: %v", evs)
	}
	// Verbatim field-presence: pipeline_start has no scene_id/phase keys.
	if _, has := first["scene_id"]; has {
		t.Error("pipeline_start grew a scene_id key — events must be unaltered")
	}

	if msgs[0].event != "run-state" {
		t.Errorf("first frame should be run-state, got %q", msgs[0].event)
	}
	lastFrames := msgs[len(msgs)-2:]
	if lastFrames[0].event != "run-state" || lastFrames[1].event != "stream-end" {
		t.Errorf("stream should end with run-state + stream-end, got %+v", lastFrames)
	}
	var finalState runStatePayload
	if err := json.Unmarshal([]byte(lastFrames[0].data), &finalState); err != nil {
		t.Fatal(err)
	}
	if finalState.State != StateSucceeded {
		t.Errorf("final run-state = %s, want succeeded", finalState.State)
	}
}

func TestSSE_CancelEndsStream(t *testing.T) {
	s, fe := launchServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "1"})
	waitStart(t, fe)
	resp, err := http.Get(ts.URL + "/api/runs/" + id + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		doJSON(t, s.Handler(), http.MethodPost, "/api/runs/"+id+"/cancel", nil)
	}()
	msgs := readSSE(t, resp.Body, 3*time.Second)
	if len(msgs) == 0 || msgs[len(msgs)-1].event != "stream-end" {
		t.Fatalf("cancelled run stream must end with stream-end: %+v", msgs)
	}
	var st runStatePayload
	json.Unmarshal([]byte(msgs[len(msgs)-2].data), &st)
	if st.State != StateCancelled {
		t.Errorf("final state = %s, want cancelled", st.State)
	}
}

func TestSSE_FinishedRunReplaysVerbatim(t *testing.T) {
	s, fe := launchServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "1"})
	waitStart(t, fe)
	fe.chanFor(id) <- true
	waitState(t, s, id, StateSucceeded)

	// The run is gone from memory: this replays from the database.
	stored, err := s.rm.hist.listEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 stored events, got %d", len(stored))
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s/events", ts.URL, id))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	msgs := readSSE(t, resp.Body, 3*time.Second)
	evs := pipelineEvents(msgs)
	if len(evs) != len(stored) {
		t.Fatalf("replayed %d events, stored %d", len(evs), len(stored))
	}
	for i := range evs {
		if evs[i] != stored[i] {
			t.Errorf("replayed event %d differs from stored line:\nreplay: %s\nstored: %s", i, evs[i], stored[i])
		}
	}
	if msgs[len(msgs)-1].event != "stream-end" {
		t.Error("historical replay must end with stream-end")
	}
}

func TestSSE_FailedRunStreamsErrorEventThenEnd(t *testing.T) {
	s, fe := launchServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "1"})
	waitStart(t, fe)
	resp, err := http.Get(ts.URL + "/api/runs/" + id + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	go func() {
		time.Sleep(30 * time.Millisecond)
		fe.chanFor(id) <- false // emits error event, returns err
	}()
	msgs := readSSE(t, resp.Body, 3*time.Second)
	evs := pipelineEvents(msgs)
	var last map[string]any
	if err := json.Unmarshal([]byte(evs[len(evs)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if last["type"] != "error" || !strings.Contains(last["message"].(string), "boom") {
		t.Errorf("last pipeline event should be the error event: %v", last)
	}
	if msgs[len(msgs)-1].event != "stream-end" {
		t.Error("failed run stream must end with stream-end")
	}
}
