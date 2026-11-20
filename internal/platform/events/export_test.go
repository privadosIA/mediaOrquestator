package events

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// parseLines decodes a JSON-lines stream into events, failing the test on
// any malformed line (the existing consumer contract must keep parsing).
func parseLines(t *testing.T, buf *bytes.Buffer) []Event {
	t.Helper()
	var out []Event
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %q is not a valid event: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

// TestExportEvents_ImagesModeStream simulates the event sequence the
// orchestrator emits for one images-mode scene (render + export pairs, plus
// the global export step) and asserts the stream honours the existing
// contract: pairs per phase that runs, no events for skipped phases, and no
// new fields or types for the export phase.
func TestExportEvents_ImagesModeStream(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)

	e.PipelineStart()
	e.PhaseStart("intro", "render")
	e.PhaseEnd("intro", "render", "tmp/scene_intro.png")
	e.PhaseStart("intro", "export")
	e.PhaseEnd("intro", "export", "out/my_project/intro.png")
	e.PhaseStart("_global", "export")
	e.PhaseEnd("_global", "export", "out/my_project/manifest.json")
	e.Done("out/my_project/manifest.json")

	evs := parseLines(t, &buf)

	// No tts/probe/clip/concat events anywhere in the stream.
	for _, ev := range evs {
		switch ev.Phase {
		case "tts", "probe", "clip", "concat":
			t.Errorf("skipped phase %q must emit no events, got %+v", ev.Phase, ev)
		}
	}

	// Every phase_start is paired with a phase_end for the same scene/phase.
	open := map[string]int{}
	for _, ev := range evs {
		key := ev.SceneID + "/" + ev.Phase
		switch ev.Type {
		case TypePhaseStart:
			open[key]++
		case TypePhaseEnd:
			open[key]--
			if open[key] < 0 {
				t.Errorf("phase_end without matching phase_start for %s", key)
			}
		}
	}
	for key, n := range open {
		if n != 0 {
			t.Errorf("unpaired phase_start for %s", key)
		}
	}

	// The export phase_end carries the artifact path in the existing field.
	var sawSceneExport, sawGlobalExport bool
	for _, ev := range evs {
		if ev.Type == TypePhaseEnd && ev.Phase == "export" {
			switch ev.SceneID {
			case "intro":
				sawSceneExport = true
				if ev.Artifact != "out/my_project/intro.png" {
					t.Errorf("scene export artifact = %q", ev.Artifact)
				}
			case "_global":
				sawGlobalExport = true
				if ev.Artifact != "out/my_project/manifest.json" {
					t.Errorf("global export artifact = %q", ev.Artifact)
				}
			}
		}
	}
	if !sawSceneExport || !sawGlobalExport {
		t.Errorf("expected per-scene and global export pairs (scene=%v global=%v)", sawSceneExport, sawGlobalExport)
	}
}

// TestExportEvents_ShapeUnchanged pins that an export event marshals with
// exactly the existing field set, so pre-change JSON-lines consumers parse
// it without error.
func TestExportEvents_ShapeUnchanged(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, true)
	e.PhaseStart("intro", "export")
	e.PhaseEnd("intro", "export", "out/p/intro.png")

	allowed := map[string]bool{
		"timestamp": true, "type": true, "scene_id": true, "phase": true,
		"duration_ms": true, "artifact": true, "message": true,
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		for field := range raw {
			if !allowed[field] {
				t.Errorf("export event introduces new field %q: %s", field, line)
			}
		}
	}
}
