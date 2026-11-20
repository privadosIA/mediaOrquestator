package config

import (
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestEffectiveRender_SceneWinsAndAbsentFallsBackToTheProject(t *testing.T) {
	d := Defaults{Resolution: "1024x576", Steps: 8, ModelSet: &ModelSet{CfgScale: f64(1)}}
	if r := (Scene{}).EffectiveRender(d); r.Resolution != "1024x576" || r.Steps != 8 || r.CfgScale == nil || *r.CfgScale != 1 {
		t.Fatalf("absent block = %+v, want the project's", r)
	}
	s := Scene{Render: &SceneRender{Resolution: "1280x704", Steps: 12, CfgScale: f64(0)}}
	r := s.EffectiveRender(d)
	if r.Resolution != "1280x704" || r.Steps != 12 || r.CfgScale == nil || *r.CfgScale != 0 {
		t.Fatalf("scene block = %+v, want 1280x704 / 12 / cfg 0 (0 is a value, not absent)", r)
	}
	if r := (Scene{Render: &SceneRender{Steps: 4}}).EffectiveRender(d); r.Resolution != "1024x576" || r.Steps != 4 {
		t.Fatalf("partial block = %+v, want only the steps replaced", r)
	}
}

func TestEffectiveRender_NoCfgWithASingleCheckpointUnlessTheSceneSetsOne(t *testing.T) {
	d := Defaults{Model: "m.safetensors", Resolution: "512x512", Steps: 20}
	if r := (Scene{}).EffectiveRender(d); r.CfgScale != nil {
		t.Fatalf("cfg = %v, want none (sd uses its own)", *r.CfgScale)
	}
	if r := (Scene{Render: &SceneRender{CfgScale: f64(3)}}).EffectiveRender(d); r.CfgScale == nil || *r.CfgScale != 3 {
		t.Fatalf("scene cfg = %v, want 3", r.CfgScale)
	}
}

func TestAnimateForScene_SceneValuesLaidOverTheProjectsAndDefaultsFilled(t *testing.T) {
	a := Animate{DiffusionModel: "w", T5XXL: "t", VAE: "v", Frames: 33}
	got := a.ForScene(Scene{Animation: &SceneAnimation{Frames: 49, Steps: 12}})
	if got.Frames != 49 || got.Steps != 12 || got.Resolution != DefaultAnimateResolution || got.FPS != DefaultAnimateFPS {
		t.Fatalf("ForScene = %+v", got)
	}
	if got := a.ForScene(Scene{}); got.Frames != 33 || got.Steps != DefaultAnimateSteps {
		t.Fatalf("no animation block = %+v, want the project's", got)
	}
	// The two-expert defaults still apply when the scene sets only some values.
	two := Animate{DiffusionModel: "w", HighNoiseDiffusionModel: "h"}
	if got := two.ForScene(Scene{Animation: &SceneAnimation{Frames: 17}}); got.Steps != DefaultAnimateTwoExpertSteps || got.Frames != 17 {
		t.Fatalf("two-expert ForScene = %+v", got)
	}
}

func TestValidateSceneQuality(t *testing.T) {
	bad := []struct {
		name string
		s    Scene
		want string
	}{
		{"resolution", Scene{ID: "a", Render: &SceneRender{Resolution: "big"}}, `scene "a": render.resolution`},
		{"zero side", Scene{ID: "a", Render: &SceneRender{Resolution: "0x576"}}, "render.resolution"},
		{"steps", Scene{ID: "a", Render: &SceneRender{Steps: -1}}, "render.steps"},
		{"cfg", Scene{ID: "a", Render: &SceneRender{CfgScale: f64(-1)}}, "render.cfg_scale"},
		{"anim resolution", Scene{ID: "a", Animation: &SceneAnimation{Resolution: "830x480"}}, "multiples of 16"},
		{"anim frames", Scene{ID: "a", Animation: &SceneAnimation{Frames: 30}}, "animation.frames must be 4n+1"},
		{"anim steps", Scene{ID: "a", Animation: &SceneAnimation{Steps: -2}}, "animation.steps"},
	}
	for _, c := range bad {
		errs := validateSceneQuality(c.s, EngineWan)
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), c.want) {
			t.Errorf("%s: errors = %v, want one containing %q", c.name, errs, c.want)
		}
	}
	good := Scene{ID: "a", Render: &SceneRender{Resolution: "1280x704", Steps: 12, CfgScale: f64(0)}, Animation: &SceneAnimation{Resolution: "832x480", Frames: 49, Steps: 10}}
	if errs := validateSceneQuality(good, EngineWan); len(errs) != 0 {
		t.Errorf("valid settings rejected: %v", errs)
	}
	if errs := validateSceneQuality(Scene{ID: "a"}, EngineWan); len(errs) != 0 {
		t.Errorf("no block must be valid: %v", errs)
	}
}

func TestScenesYAML_RoundTripsTheBlocksAndLeavesThemOutWhenAbsent(t *testing.T) {
	cfg, err := LoadBytes([]byte(`
project: p
output: {file: out/p.mp4}
defaults: {model: m.safetensors, voice: v.onnx, resolution: 1024x576, sampler: euler, steps: 8, fps: 24}
scenes:
  - id: a
    prompt: x
    tts_text: hi
    speed: 1
    render: {resolution: 1280x704, steps: 12}
    animation: {frames: 49}
  - id: b
    prompt: y
    tts_text: yo
    speed: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if r := cfg.Scenes[0].Render; r == nil || r.Resolution != "1280x704" || r.Steps != 12 {
		t.Fatalf("render = %+v", r)
	}
	if a := cfg.Scenes[0].Animation; a == nil || a.Frames != 49 {
		t.Fatalf("animation = %+v", a)
	}
	if cfg.Scenes[1].Render != nil || cfg.Scenes[1].Animation != nil {
		t.Fatalf("scene b must have no blocks")
	}
}
