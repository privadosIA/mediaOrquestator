package config

import (
	"strings"
	"testing"
)

func ltxConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	c := animateConfig(t)
	c.Defaults.Animate = &Animate{
		Engine:         EngineLTX,
		DiffusionModel: touchFile(t, dir, "dit.gguf"),
		VAE:            touchFile(t, dir, "vae.safetensors"),
		AudioVAE:       touchFile(t, dir, "audio_vae.safetensors"),
		LLM:            touchFile(t, dir, "gemma.gguf"),
		Connectors:     touchFile(t, dir, "connectors.safetensors"),
	}
	return c
}

func TestLTX_ResolvedDefaults(t *testing.T) {
	a := (Animate{Engine: EngineLTX, FPS: 8}).Resolved()
	if a.Resolution != "768x416" || a.Frames != 49 || a.FPS != 24 || a.Steps != 8 || *a.CfgScale != 1 {
		t.Errorf("defaults = %+v (cfg %v)", a, *a.CfgScale)
	}
}

func TestLTX_ValidWithoutWanFiles(t *testing.T) {
	if err := ltxConfig(t).Validate(); err != nil {
		t.Errorf("valid LTX-2.3 block refused: %v", err)
	}
}

func TestLTX_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		mut  func(*Config)
		want string
	}{
		"frames off the grid": {func(c *Config) { c.Defaults.Animate.Frames = 56 }, "8k+1"},
		"resolution":          {func(c *Config) { c.Defaults.Animate.Resolution = "768x432" }, "multiples of 32"},
		"no connectors":       {func(c *Config) { c.Defaults.Animate.Connectors = "" }, "defaults.animate.connectors is not set"},
		"no llm":              {func(c *Config) { c.Defaults.Animate.LLM = "" }, "defaults.animate.llm is not set"},
		"no audio vae":        {func(c *Config) { c.Defaults.Animate.AudioVAE = "" }, "defaults.animate.audio_vae is not set"},
		"missing dit":         {func(c *Config) { c.Defaults.Animate.DiffusionModel += ".nope" }, "defaults.animate.diffusion_model"},
		"lora":                {func(c *Config) { c.Defaults.Animate.LoRA = "x.safetensors" }, "lora does not apply"},
		"first frame":         {func(c *Config) { c.Defaults.Animate.FirstFrame = true }, "only applies to the minimax-h3"},
		"scene frames":        {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Frames: 56} }, "8k+1"},
		"scene resolution":    {func(c *Config) { c.Scenes[0].Animation = &SceneAnimation{Resolution: "770x416"} }, "multiples of 32"},
	} {
		t.Run(name, func(t *testing.T) {
			c := ltxConfig(t)
			tc.mut(c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLTX_FrameGridAccepted(t *testing.T) {
	for _, n := range []int{9, 33, 49, 57, 121} {
		c := ltxConfig(t)
		c.Defaults.Animate.Frames = n
		if err := c.Validate(); err != nil {
			t.Errorf("frames %d refused: %v", n, err)
		}
	}
}

func TestLTX_ConnectorsOnlyWithLTX(t *testing.T) {
	c := animateConfig(t)
	c.Defaults.Animate.Connectors = "x.safetensors"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "only applies to the ltx-2.3") {
		t.Errorf("want connectors refusal, got %v", err)
	}
}
