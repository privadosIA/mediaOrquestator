// Package events emits pipeline progress as a single stream of structured
// events, either as human-readable text or as JSON-lines (one JSON object
// per line) so a front-end can tail stdout and render live progress.
//
// The JSON-lines contract is transcribed verbatim in docs/events.md; any
// field added or renamed here must be reflected there too.
package events

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Type enumerates the kinds of events the pipeline emits.
type Type string

// The events of a run, in the order a run emits them: it starts, each phase starts and ends (or a scene is skipped),
// warnings and errors may come between, and the run ends.
const (
	TypePipelineStart Type = "pipeline_start"
	TypePhaseStart    Type = "phase_start"
	TypePhaseEnd      Type = "phase_end"
	TypeSkipped       Type = "skipped"
	TypeWarn          Type = "warn"
	TypeError         Type = "error"
	TypePipelineEnd   Type = "pipeline_end"
)

// Event is the single JSON object emitted per line in --json mode.
type Event struct {
	Timestamp  string `json:"timestamp"`
	Type       Type   `json:"type"`
	SceneID    string `json:"scene_id,omitempty"`
	Phase      string `json:"phase,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	// LoadMs is, on a phase_end of a phase that runs sd, the milliseconds sd spent loading model weights (summed over
	// the invocations of the phase). Omitted when it could not be read, never larger than DurationMs.
	LoadMs   int64  `json:"load_ms,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Emitter writes pipeline progress to w, either as JSON-lines (jsonMode)
// or as human-readable text, and tracks phase start times so PhaseEnd can
// report elapsed duration.
type Emitter struct {
	w           io.Writer
	jsonMode    bool
	mu          sync.Mutex
	phaseStarts map[string]time.Time
	pipelineAt  time.Time
}

// New creates an Emitter writing to w. jsonMode selects JSON-lines output;
// otherwise events are rendered as human-readable text.
func New(w io.Writer, jsonMode bool) *Emitter {
	return &Emitter{
		w:           w,
		jsonMode:    jsonMode,
		phaseStarts: make(map[string]time.Time),
		pipelineAt:  time.Now(),
	}
}

func phaseKey(sceneID, phase string) string { return sceneID + "\x00" + phase }

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (e *Emitter) emit(ev Event) {
	ev.Timestamp = timestamp()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.jsonMode {
		line, err := json.Marshal(ev)
		if err != nil {
			return
		}
		fmt.Fprintln(e.w, string(line))
		return
	}
	fmt.Fprintln(e.w, formatText(ev))
}

func formatText(ev Event) string {
	switch ev.Type {
	case TypePipelineStart:
		return fmt.Sprintf("[%s] pipeline start", ev.Timestamp)
	case TypePhaseStart:
		return fmt.Sprintf("[%s] %-6s start  scene=%s", ev.Timestamp, ev.Phase, ev.SceneID)
	case TypePhaseEnd:
		s := fmt.Sprintf("[%s] %-6s done   scene=%s (%dms)", ev.Timestamp, ev.Phase, ev.SceneID, ev.DurationMs)
		if ev.LoadMs > 0 {
			s += fmt.Sprintf(" load=%dms", ev.LoadMs)
		}
		if ev.Artifact != "" {
			s += " artifact=" + ev.Artifact
		}
		return s
	case TypeSkipped:
		return fmt.Sprintf("[%s] %-6s skip   scene=%s (cached)", ev.Timestamp, ev.Phase, ev.SceneID)
	case TypeWarn:
		return fmt.Sprintf("[%s] WARN scene=%s: %s", ev.Timestamp, ev.SceneID, ev.Message)
	case TypeError:
		return fmt.Sprintf("[%s] ERROR scene=%s: %s", ev.Timestamp, ev.SceneID, ev.Message)
	case TypePipelineEnd:
		return fmt.Sprintf("[%s] pipeline end -> %s (%dms)", ev.Timestamp, ev.Artifact, ev.DurationMs)
	default:
		return fmt.Sprintf("[%s] %s", ev.Timestamp, ev.Type)
	}
}

// PipelineStart emits the single event that opens the stream.
func (e *Emitter) PipelineStart() {
	e.emit(Event{Type: TypePipelineStart})
}

// PhaseStart records the phase's start time and emits phase_start.
func (e *Emitter) PhaseStart(sceneID, phase string) {
	e.mu.Lock()
	e.phaseStarts[phaseKey(sceneID, phase)] = time.Now()
	e.mu.Unlock()
	e.emit(Event{Type: TypePhaseStart, SceneID: sceneID, Phase: phase})
}

// PhaseEnd emits phase_end with the elapsed duration since the matching
// PhaseStart and the path of the artifact the phase produced (may be "").
func (e *Emitter) PhaseEnd(sceneID, phase, artifact string) {
	e.PhaseEndLoad(sceneID, phase, artifact, 0)
}

// PhaseEndLoad is PhaseEnd for a phase that ran sd, with the milliseconds sd spent loading models (0 when it could
// not be read: the field is then omitted). The load never exceeds the duration of the phase.
func (e *Emitter) PhaseEndLoad(sceneID, phase, artifact string, loadMs int) {
	e.mu.Lock()
	start, ok := e.phaseStarts[phaseKey(sceneID, phase)]
	delete(e.phaseStarts, phaseKey(sceneID, phase))
	e.mu.Unlock()

	var dur int64
	if ok {
		dur = time.Since(start).Milliseconds()
	}
	load := int64(loadMs)
	if load > dur {
		load = dur
	}
	if load < 0 {
		load = 0
	}
	e.emit(Event{Type: TypePhaseEnd, SceneID: sceneID, Phase: phase, DurationMs: dur, LoadMs: load, Artifact: artifact})
}

// Skipped reports that a phase was a cache hit and was not re-run.
func (e *Emitter) Skipped(sceneID, phase string) {
	e.emit(Event{Type: TypeSkipped, SceneID: sceneID, Phase: phase})
}

// Warn reports a non-fatal condition (e.g. low free VRAM).
func (e *Emitter) Warn(sceneID, message string) {
	e.emit(Event{Type: TypeWarn, SceneID: sceneID, Message: message})
}

// Error reports a fatal condition. The caller is expected to exit non-zero
// immediately after.
func (e *Emitter) Error(sceneID, message string) {
	e.emit(Event{Type: TypeError, SceneID: sceneID, Message: message})
}

// Done emits the closing pipeline_end event, naming the final assembled
// video and the total wall-clock duration since the Emitter was created.
func (e *Emitter) Done(finalPath string) {
	e.emit(Event{Type: TypePipelineEnd, Artifact: finalPath, DurationMs: time.Since(e.pipelineAt).Milliseconds()})
}
