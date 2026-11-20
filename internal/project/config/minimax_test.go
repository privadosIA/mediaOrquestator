package config

import (
	"strings"
	"testing"
)

func miniMaxConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	c := animateConfig(t)
	c.Defaults.Animate = &Animate{
		Engine:         EngineMiniMaxH3,
		DiffusionModel: touchFile(t, dir, "dit.gguf"),
		VAE:            touchFile(t, dir, "vae.safetensors"),
		AudioVAE:       touchFile(t, dir, "audio_vae.safetensors"),
		LLM:            touchFile(t, dir, "llm.gguf"),
	}
	return c
}

func TestMiniMax_ResolvedDefaults(t *testing.T) {
	a := (Animate{Engine: EngineMiniMaxH3, FPS: 8}).Resolved()
	if a.Resolution != "864x480" || a.Frames != 56 || a.FPS != 24 || a.Steps != 20 || *a.CfgScale != 1 {
		t.Errorf("defaults = %+v (cfg %v)", a, *a.CfgScale)
	}
}

func TestMiniMax_ValidWithoutWanFiles(t *testing.T) {
	if err := miniMaxConfig(t).Validate(); err != nil {
		t.Errorf("valid MiniMax-H3 block refused: %v", err)
	}
}

func TestMiniMax_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		mut  func(*Config)
		want string
	}{
		"frames off the grid": {func(c *Config) { c.Defaults.Animate.Frames = 33 }, "17k+5"},
		"resolution":          {func(c *Config) { c.Defaults.Animate.Resolution = "848x480" }, "multiples of 32"},
		"no llm":              {func(c *Config) { c.Defaults.Animate.LLM = "" }, "defaults.animate.llm is not set"},
		"no audio vae":        {func(c *Config) { c.Defaults.Animate.AudioVAE = "" }, "defaults.animate.audio_vae is not set"},
		"missing dit":         {func(c *Config) { c.Defaults.Animate.DiffusionModel += ".nope" }, "defaults.animate.diffusion_model"},
		"scene frames":        {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Frames: 33} }, "17k+5"},
		"scene resolution":    {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Resolution: "850x480"} }, "multiples of 32"},
	} {
		t.Run(name, func(t *testing.T) {
			c := miniMaxConfig(t)
			tc.mut(c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestMiniMax_FrameGridAccepted(t *testing.T) {
	c := miniMaxConfig(t)
	c.Defaults.Animate.Frames = 22
	c.Scenes[0].Animation = &SceneAnimation{Frames: 39, Resolution: "704x416"}
	if err := c.Validate(); err != nil {
		t.Errorf("22 and 39 frames at 704x416 must be accepted: %v", err)
	}
}

func TestMiniMax_FirstFrameDefaultsToEightStepsWithItsLoRAAndOnlyAppliesToH3(t *testing.T) {
	a := (Animate{Engine: EngineMiniMaxH3, FirstFrame: true, LoRA: "l.safetensors"}).Resolved()
	if a.Steps != 8 {
		t.Errorf("steps = %d, want 8 for the first-frame LoRA", a.Steps)
	}
	if (Animate{Engine: EngineMiniMaxH3, LoRA: "l.safetensors"}).Resolved().Steps != 4 {
		t.Error("the reference LoRA keeps its 4 steps")
	}
	if (Animate{Engine: EngineMiniMaxH3, FirstFrame: true}).Resolved().Steps != 20 {
		t.Error("without a LoRA the steps stay at 20")
	}
	c := animateConfig(t)
	c.Defaults.Animate = &Animate{FirstFrame: true, DiffusionModel: "x"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "first_frame only applies to the minimax-h3 engine") {
		t.Errorf("err = %v", err)
	}
}
