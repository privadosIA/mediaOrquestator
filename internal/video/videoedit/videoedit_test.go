package videoedit

import (
	"context"
	"strings"
	"testing"
)

func TestTools_AreListedInTheStableOrderWithDistinctIdentifiers(t *testing.T) {
	want := []string{"scale", "interpolate", "reframe", "remove-background", "depth", "extend", "motion-control"}
	got := IDs()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, tool := range Tools() {
		if seen[tool.ID] || tool.Name == "" || tool.Description == "" {
			t.Errorf("tool %q is repeated or has no name or description", tool.ID)
		}
		seen[tool.ID] = true
	}
	if m, _ := Lookup(MotionControl); !m.NeedsImage || !m.AcceptsMotion {
		t.Errorf("motion-control must need a picture and take a motion: %+v", m)
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("an unknown tool was found")
	}
}

func TestValidateSettings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tool     string
		settings map[string]any
		wantErr  string
	}{
		{"scale defaults", Scale, nil, ""},
		{"scale x4 quality", Scale, map[string]any{"factor": 4.0, "engine": "quality"}, ""},
		{"scale factor 3", Scale, map[string]any{"factor": 3.0}, "setting factor"},
		{"scale engine unknown", Scale, map[string]any{"engine": "magic"}, "setting engine"},
		{"unknown setting", Scale, map[string]any{"speed": 2.0}, `"speed" is not one of the tool`},
		{"interpolate 60", Interpolate, map[string]any{"fps": 60.0}, ""},
		{"interpolate 25", Interpolate, map[string]any{"fps": 25.0}, "setting fps"},
		{"reframe 4:5 pad", Reframe, map[string]any{"aspect_ratio": "4:5", "mode": "pad", "color": "#FFAA00"}, ""},
		{"reframe 21:9", Reframe, map[string]any{"aspect_ratio": "21:9"}, "setting aspect_ratio"},
		{"reframe colour", Reframe, map[string]any{"color": "red"}, "setting color"},
		{"background alpha", RemoveBackground, map[string]any{"alpha": true}, ""},
		{"background alpha text", RemoveBackground, map[string]any{"alpha": "yes"}, "setting alpha"},
		{"depth takes none", Depth, map[string]any{"x": 1.0}, "is not one of the tool"},
		{"extend without a prompt", Extend, map[string]any{"seconds": 5.0}, "prompt is required"},
		{"extend 10", Extend, map[string]any{"seconds": 10.0, "prompt": "she waves", "engine": "ltx"}, ""},
		{"extend 11", Extend, map[string]any{"seconds": 11.0, "prompt": "p"}, "above the maximum"},
		{"extend 1", Extend, map[string]any{"seconds": 1.0, "prompt": "p"}, "below the minimum"},
		{"extend 2.5", Extend, map[string]any{"seconds": 2.5, "prompt": "p"}, "whole number"},
		{"extend engine", Extend, map[string]any{"engine": "wan", "prompt": "p"}, "setting engine"},
		{"motion strength 0.1", MotionControl, map[string]any{"strength": 0.1}, "below the minimum"},
		{"motion strength 1.1", MotionControl, map[string]any{"strength": 1.1}, "above the maximum"},
		{"motion guidance 0.5", MotionControl, map[string]any{"guidance": 0.5}, "below the minimum"},
		{"motion guidance 11", MotionControl, map[string]any{"guidance": 11.0}, "above the maximum"},
		{"motion guidance 4", MotionControl, map[string]any{"guidance": 4.0}, ""},
		{"motion start negative", MotionControl, map[string]any{"start": -1.0}, "below the minimum"},
		{"motion ok", MotionControl, map[string]any{"strength": 1.0, "start": 3.5, "prompt": "a dancer"}, ""},
		{"prompt too long", MotionControl, map[string]any{"prompt": strings.Repeat("a", MaxPrompt+1)}, "longer than"},
		{"unknown tool", "teleport", nil, "is unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateSettings(tc.tool, tc.settings)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error = %v, want one with %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSettings_FillsTheDefaultsAndNormalises(t *testing.T) {
	got, err := ValidateSettings(Scale, nil)
	if err != nil {
		t.Fatal(err)
	}
	if Int(got, "factor") != 2 || Str(got, "engine") != "fast" {
		t.Errorf("defaults = %v", got)
	}
	got, err = ValidateSettings(Reframe, map[string]any{"color": "#AbCdEf", "mode": "pad"})
	if err != nil {
		t.Fatal(err)
	}
	if Str(got, "color") != "#abcdef" || Str(got, "aspect_ratio") != "16:9" {
		t.Errorf("normalised = %v", got)
	}
	got, _ = ValidateSettings(MotionControl, map[string]any{})
	if Float(got, "strength") != 0.6 || Float(got, "start") != 0 {
		t.Errorf("motion defaults = %v", got)
	}
	// an integer written as a Go int (from a YAML file) is accepted like a JSON number
	if _, err := ValidateSettings(Scale, map[string]any{"factor": 2}); err != nil {
		t.Errorf("int factor: %v", err)
	}
}

func TestAvailability_NoCodeNoTool(t *testing.T) {
	c := NewBareCatalog()
	if ok, why := c.AvailabilityOf(Depth); ok || why == "" {
		t.Errorf("a tool without code must be unavailable with a reason, got %v %q", ok, why)
	}
	c.Register(Depth, Impl{Run: nil})
	c.Register(Depth, Impl{Run: noRun, Available: func() (bool, string) { return false, "the model is missing" }})
	if ok, why := c.AvailabilityOf(Depth); ok || why != "the model is missing" {
		t.Errorf("availability = %v %q", ok, why)
	}
	c.Register(Depth, Impl{Run: noRun})
	if ok, _ := c.AvailabilityOf(Depth); !ok {
		t.Error("a tool with code and no condition must be available")
	}
}

func TestParseProbe(t *testing.T) {
	info, err := parseProbe([]byte(`{"streams":[{"codec_type":"video","width":832,"height":480,"avg_frame_rate":"16/1","r_frame_rate":"16/1","duration":"2.0"},{"codec_type":"audio"}],"format":{"duration":"2.05"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 832 || info.Height != 480 || info.FPS != 16 || info.Duration != 2 || !info.HasAudio {
		t.Errorf("info = %+v", info)
	}
	info, err = parseProbe([]byte(`{"streams":[{"codec_type":"video","width":10,"height":10,"avg_frame_rate":"0/0","r_frame_rate":"30000/1001"}],"format":{"duration":"3.5"}}`))
	if err != nil || info.Duration != 3.5 || info.HasAudio || int(info.FPS*100) != 2997 {
		t.Errorf("info = %+v, %v", info, err)
	}
	if _, err := parseProbe([]byte(`{"streams":[{"codec_type":"audio"}]}`)); err == nil {
		t.Error("a file with no video stream was accepted")
	}
	if _, err := parseProbe([]byte(`nope`)); err == nil {
		t.Error("garbage was accepted")
	}
}

func noRun(_ context.Context, _ Job) (Result, error) { return Result{}, nil }

// The catalog of a build has code for every tool it lists, and a bare one has none: the tools were registered by init
// functions before, and a lost registration only showed when somebody used the tool.
func TestNewCatalog_HasTheCodeOfEveryTool(t *testing.T) {
	c := NewCatalog()
	for _, id := range IDs() {
		if _, ok := c.ImplOf(id); !ok {
			t.Errorf("the tool %s has no code in the catalog of this build", id)
		}
	}
	if got := len(c.IDsWithCode()); got != len(IDs()) {
		t.Errorf("%d tools with code, want %d", got, len(IDs()))
	}
	if n := len(NewBareCatalog().IDsWithCode()); n != 0 {
		t.Errorf("a bare catalog has %d tools", n)
	}
}
