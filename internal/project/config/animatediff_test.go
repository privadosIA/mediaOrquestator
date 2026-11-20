package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func animateDiffConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	c := animateConfig(t)
	c.Defaults.Animate = &Animate{
		Engine:       EngineAnimateDiff,
		Checkpoint:   touchFile(t, dir, "sd15.safetensors"),
		MotionModule: touchFile(t, dir, "mm_sd15_v3.safetensors"),
	}
	return c
}

func TestAnimateDiff_ResolvedDefaults(t *testing.T) {
	a := (Animate{Engine: EngineAnimateDiff}).Resolved()
	if a.Resolution != "512x512" || a.Frames != 16 || a.FPS != 8 || a.Steps != 25 ||
		*a.CfgScale != 7 || *a.Strength != 0.75 {
		t.Errorf("resolved = %+v", a)
	}
}

func TestAnimateDiff_UserWins(t *testing.T) {
	zero, strength := 0.0, 0.6
	a := (Animate{Engine: EngineAnimateDiff, Frames: 24, Strength: &strength, CfgScale: &zero}).Resolved()
	if a.Frames != 24 || *a.Strength != 0.6 || *a.CfgScale != 0 {
		t.Errorf("resolved = %+v", a)
	}
}

func TestAnimate_WanUnchangedWhenEngineAbsent(t *testing.T) {
	for _, engine := range []string{"", EngineWan} {
		a := (Animate{Engine: engine, DiffusionModel: "w", T5XXL: "t", VAE: "v"}).Resolved()
		if a.Resolution != "832x480" || a.Frames != 33 || a.FPS != 16 || a.Steps != 20 || *a.CfgScale != 6 || a.Strength != nil {
			t.Errorf("engine %q: resolved = %+v", engine, a)
		}
	}
	out, err := yaml.Marshal(Animate{DiffusionModel: "w", T5XXL: "t", VAE: "v"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "diffusion_model: w\nt5xxl: t\nvae: v\n" {
		t.Errorf("a Wan block serializes as before, got:\n%s", got)
	}
}

func TestAnimateDiff_ValidWithoutWanFiles(t *testing.T) {
	if err := animateDiffConfig(t).Validate(); err != nil {
		t.Errorf("valid AnimateDiff block refused: %v", err)
	}
}

func TestAnimateDiff_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		mut  func(*Config)
		want string
	}{
		"unknown engine":   {func(c *Config) { c.Defaults.Animate.Engine = "svd" }, `must be "wan", "animatediff", "minimax-h3" or "ltx-2.3"`},
		"frames too many":  {func(c *Config) { c.Defaults.Animate.Frames = 40 }, "between 8 and 32"},
		"frames too few":   {func(c *Config) { c.Defaults.Animate.Frames = 4 }, "between 8 and 32"},
		"resolution":       {func(c *Config) { c.Defaults.Animate.Resolution = "500x512" }, "multiples of 8"},
		"strength zero":    {func(c *Config) { z := 0.0; c.Defaults.Animate.Strength = &z }, "strength"},
		"strength over 1":  {func(c *Config) { v := 1.5; c.Defaults.Animate.Strength = &v }, "strength"},
		"no checkpoint":    {func(c *Config) { c.Defaults.Animate.Checkpoint = "" }, "defaults.animate.checkpoint is not set"},
		"missing module":   {func(c *Config) { c.Defaults.Animate.MotionModule += ".nope" }, "defaults.animate.motion_module"},
		"scene frames":     {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Frames: 49} }, "between 8 and 32"},
		"scene resolution": {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Resolution: "514x512"} }, "multiples of 8"},
	} {
		t.Run(name, func(t *testing.T) {
			c := animateDiffConfig(t)
			tc.mut(c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAnimateDiff_FramesNotFourNPlusOne(t *testing.T) {
	c := animateDiffConfig(t)
	c.Defaults.Animate.Frames = 16
	c.Scenes[0].Animation = &SceneAnimation{Frames: 12, Resolution: "384x384"}
	if err := c.Validate(); err != nil {
		t.Errorf("16 and 12 frames must be accepted for AnimateDiff: %v", err)
	}
	// ... and still refused for Wan.
	w := animateConfig(t)
	w.Defaults.Animate.Frames = 16
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "4n+1") {
		t.Errorf("Wan must keep refusing 16 frames: %v", err)
	}
}

// The block documented in docs/SCENES_YAML.md parses with its fields.
func TestLoad_AnimateDiffParses(t *testing.T) {
	p := writeTempYAML(t, `project: demo
output_file: out.mp4
defaults:
  animate:
    engine: animatediff
    checkpoint: models/v1-5-pruned-emaonly.safetensors
    motion_module: models/video/mm_sd15_v3.safetensors
    strength: 0.6
scenes:
  - id: a
    prompt: p
    tts_text: t
    animate: true
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Defaults.Animate
	if a == nil || !a.IsAnimateDiff() || a.Checkpoint != "models/v1-5-pruned-emaonly.safetensors" ||
		a.MotionModule != "models/video/mm_sd15_v3.safetensors" || a.Strength == nil || *a.Strength != 0.6 {
		t.Errorf("animate = %+v", a)
	}
}
