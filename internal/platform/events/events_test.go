package events

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJSONMode_EmitsOneValidJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)

	e.PipelineStart()
	e.PhaseStart("s1", "tts")
	e.PhaseEnd("s1", "tts", "tmp/scene_s1.wav")
	e.Skipped("s1", "render")
	e.Warn("_global", "low VRAM")
	e.Error("s1", "boom")
	e.Done("out/final.mp4")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	wantTypes := []Type{
		TypePipelineStart, TypePhaseStart, TypePhaseEnd, TypeSkipped,
		TypeWarn, TypeError, TypePipelineEnd,
	}
	if len(lines) != len(wantTypes) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(wantTypes), lines)
	}
	for i, line := range lines {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d: invalid JSON %q: %v", i, line, err)
		}
		if ev.Type != wantTypes[i] {
			t.Errorf("line %d: type = %q, want %q", i, ev.Type, wantTypes[i])
		}
		if ev.Timestamp == "" {
			t.Errorf("line %d: missing timestamp", i)
		}
	}
}

func TestPhaseEnd_ReportsElapsedDuration(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)

	e.PhaseStart("s1", "tts")
	e.PhaseEnd("s1", "tts", "")

	var ev Event
	line := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(line, "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &ev); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if ev.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", ev.DurationMs)
	}
}

func TestSkipped_OmitsEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)
	e.Skipped("s1", "tts")

	if strings.Contains(buf.String(), "duration_ms") {
		t.Errorf("skipped event should omit duration_ms, got: %s", buf.String())
	}
	if strings.Contains(buf.String(), "artifact") {
		t.Errorf("skipped event should omit artifact, got: %s", buf.String())
	}
}

func TestTextMode_DoesNotProduceJSON(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, false)
	e.PhaseStart("s1", "tts")

	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err == nil {
		t.Fatalf("text mode output parsed as JSON unexpectedly: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "s1") {
		t.Errorf("text output should mention scene id, got: %s", buf.String())
	}
}

func phaseEndLine(t *testing.T, e *Emitter, buf *bytes.Buffer, load int, sleep time.Duration) (Event, string) {
	t.Helper()
	buf.Reset()
	e.PhaseStart("s1", "render")
	time.Sleep(sleep)
	e.PhaseEndLoad("s1", "render", "tmp/a.png", load)
	line := strings.TrimSpace(buf.String())
	lines := strings.Split(line, "\n")
	last := lines[len(lines)-1]
	var ev Event
	if err := json.Unmarshal([]byte(last), &ev); err != nil {
		t.Fatalf("invalid JSON %q: %v", last, err)
	}
	return ev, last
}

func TestPhaseEndLoad_ReportsTheModelLoadOfThePhase(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)
	ev, _ := phaseEndLine(t, e, &buf, 5, 20*time.Millisecond)
	if ev.LoadMs != 5 || ev.DurationMs < 20 {
		t.Errorf("load_ms = %d, duration_ms = %d, want 5 and >= 20", ev.LoadMs, ev.DurationMs)
	}
}

func TestPhaseEndLoad_OmitsTheFieldWhenThereIsNoData(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)
	_, line := phaseEndLine(t, e, &buf, 0, time.Millisecond)
	if strings.Contains(line, "load_ms") {
		t.Errorf("a phase with no load time must not carry the field: %s", line)
	}
	buf.Reset()
	e.PhaseStart("s1", "tts")
	e.PhaseEnd("s1", "tts", "")
	if strings.Contains(buf.String(), "load_ms") {
		t.Errorf("PhaseEnd must not carry load_ms: %s", buf.String())
	}
}

func TestPhaseEndLoad_NeverExceedsTheDuration(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)
	ev, _ := phaseEndLine(t, e, &buf, 60_000, 5*time.Millisecond)
	if ev.LoadMs > ev.DurationMs {
		t.Errorf("load_ms %d exceeds duration_ms %d", ev.LoadMs, ev.DurationMs)
	}
	if ev, _ := phaseEndLine(t, e, &buf, -3, time.Millisecond); ev.LoadMs != 0 {
		t.Errorf("a negative load is %d, want it omitted", ev.LoadMs)
	}
}

func TestPhaseEndLoad_TextModeShowsTheLoad(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, false)
	e.PhaseStart("s1", "render")
	time.Sleep(15 * time.Millisecond)
	e.PhaseEndLoad("s1", "render", "tmp/a.png", 10)
	if !strings.Contains(buf.String(), "load=10ms") {
		t.Errorf("text = %q", buf.String())
	}
	buf.Reset()
	e.PhaseStart("s1", "tts")
	e.PhaseEnd("s1", "tts", "")
	if strings.Contains(buf.String(), "load=") {
		t.Errorf("a phase without load must not show it: %q", buf.String())
	}
}
