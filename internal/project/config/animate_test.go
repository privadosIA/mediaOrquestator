package config

import (
	"strings"
	"testing"
)

func animateConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		Project: "demo",
		Output:  Output{File: "out.mp4"},
		Defaults: Defaults{
			Resolution: "1024x576", Sampler: "euler", Steps: 8, FPS: 30,
			Animate: &Animate{
				DiffusionModel: touchFile(t, dir, "wan.gguf"),
				T5XXL:          touchFile(t, dir, "umt5.gguf"),
				VAE:            touchFile(t, dir, "wan_vae.safetensors"),
			},
		},
		Scenes: []Scene{{ID: "s1", Prompt: "p", TTSText: "t", Animate: true}},
	}
}

func TestAnimate_ResolvedDefaults(t *testing.T) {
	a := (Animate{}).Resolved()
	if a.Resolution != "832x480" || a.Frames != 33 || a.FPS != 16 || a.Steps != 20 || a.CfgScale == nil || *a.CfgScale != 6 {
		t.Errorf("resolved = %+v", a)
	}
}

func TestLoad_AnimateParses(t *testing.T) {
	p := writeTempYAML(t, `project: demo
output_file: out.mp4
defaults:
  animate:
    diffusion_model: w.gguf
    t5xxl: t.gguf
    vae: v.safetensors
    frames: 65
scenes:
  - id: a
    prompt: p
    tts_text: t
    animate: true
    motion_prompt: she turns the page
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Defaults.Animate
	if a == nil || a.DiffusionModel != "w.gguf" || a.T5XXL != "t.gguf" || a.Frames != 65 {
		t.Errorf("animate = %+v", a)
	}
	if s := cfg.Scenes[0]; !s.Animate || s.MotionPrompt != "she turns the page" {
		t.Errorf("scene = %+v", s)
	}
}

func TestValidate_Animate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"valid", func(c *Config) {}, ""},
		{"no defaults.animate", func(c *Config) { c.Defaults.Animate = nil }, "defaults.animate"},
		{"missing t5xxl", func(c *Config) { c.Defaults.Animate.T5XXL = "" }, "defaults.animate.t5xxl"},
		{"missing vae file", func(c *Config) { c.Defaults.Animate.VAE += ".nope" }, "defaults.animate.vae"},
		{"frames not 4n+1", func(c *Config) { c.Defaults.Animate.Frames = 30 }, "4n+1"},
		{"resolution not multiple of 16", func(c *Config) { c.Defaults.Animate.Resolution = "830x480" }, "multiples of 16"},
		{"files unchecked when nobody animates", func(c *Config) {
			c.Scenes[0].Animate = false
			c.Defaults.Animate.VAE = "missing.safetensors"
		}, ""},
		{"settings still checked when nobody animates", func(c *Config) {
			c.Scenes[0].Animate = false
			c.Defaults.Animate.Frames = 30
		}, "4n+1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := animateConfig(t)
			tc.mutate(cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidate_AnimateWarnings(t *testing.T) {
	cfg := animateConfig(t)
	cfg.Scenes[0].Motion = &Motion{Type: MotionZoomIn}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], `"s1"`) || !strings.Contains(cfg.Warnings[0], "motion") {
		t.Errorf("warnings = %v", cfg.Warnings)
	}

	img := animateConfig(t)
	img.Output.Mode = ModeImages
	if err := img.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(img.Warnings) != 0 { // the fields of the other modes are ignored in silence
		t.Errorf("images-mode warnings = %v", img.Warnings)
	}
}

func ptrF(v float64) *float64 { return &v }

func TestAnimate_TwoExpertDefaultsDependOnTheMode(t *testing.T) {
	single := (Animate{}).Resolved()
	if single.Steps != 20 || *single.CfgScale != 6 || single.HighNoiseSteps != 0 || single.HighNoiseCfgScale != nil {
		t.Errorf("single-model defaults changed: %+v", single)
	}
	two := (Animate{HighNoiseDiffusionModel: "hn.gguf"}).Resolved()
	if two.Steps != 10 || *two.CfgScale != 3.5 || two.HighNoiseSteps != 8 || *two.HighNoiseCfgScale != 3.5 {
		t.Errorf("two-expert defaults: %+v", two)
	}
	if two.Resolution != "832x480" || two.Frames != 33 || two.FPS != 16 {
		t.Errorf("resolution, frames and fps must keep their usual defaults: %+v", two)
	}
}

func TestAnimate_TwoExpertUserValuesWin(t *testing.T) {
	a := (Animate{
		HighNoiseDiffusionModel: "hn.gguf", Steps: 14, CfgScale: ptrF(0),
		HighNoiseSteps: 5, HighNoiseCfgScale: ptrF(0),
	}).Resolved()
	if a.Steps != 14 || *a.CfgScale != 0 || a.HighNoiseSteps != 5 || *a.HighNoiseCfgScale != 0 {
		t.Errorf("explicit values (cfg 0 included) must be kept: %+v", a)
	}
}

func TestLoad_TwoExpertParses(t *testing.T) {
	p := writeTempYAML(t, `project: demo
output_file: out.mp4
defaults:
  animate:
    diffusion_model: low.gguf
    high_noise_diffusion_model: high.gguf
    t5xxl: t.gguf
    vae: v.safetensors
    high_noise_steps: 6
    high_noise_cfg_scale: 0
scenes:
  - id: a
    prompt: p
    tts_text: t
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Defaults.Animate
	if a.HighNoiseDiffusionModel != "high.gguf" || a.HighNoiseSteps != 6 || a.HighNoiseCfgScale == nil || *a.HighNoiseCfgScale != 0 {
		t.Errorf("animate = %+v", a)
	}
}

func TestValidate_TwoExpert(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(c *Config, dir string)
		wantErr string
	}{
		{"valid two-expert", func(c *Config, dir string) {
			c.Defaults.Animate.HighNoiseDiffusionModel = touchFile(t, dir, "high.gguf")
		}, ""},
		{"steps without expert", func(c *Config, _ string) { c.Defaults.Animate.HighNoiseSteps = 8 }, "need defaults.animate.high_noise_diffusion_model"},
		{"cfg without expert", func(c *Config, _ string) { c.Defaults.Animate.HighNoiseCfgScale = ptrF(3.5) }, "need defaults.animate.high_noise_diffusion_model"},
		{"cfg 0 without expert is still refused", func(c *Config, _ string) { c.Defaults.Animate.HighNoiseCfgScale = ptrF(0) }, "need defaults.animate.high_noise_diffusion_model"},
		{"negative steps", func(c *Config, dir string) {
			c.Defaults.Animate.HighNoiseDiffusionModel = touchFile(t, dir, "high.gguf")
			c.Defaults.Animate.HighNoiseSteps = -2
		}, "high_noise_steps must be positive"},
		{"negative cfg", func(c *Config, dir string) {
			c.Defaults.Animate.HighNoiseDiffusionModel = touchFile(t, dir, "high.gguf")
			c.Defaults.Animate.HighNoiseCfgScale = ptrF(-1)
		}, "high_noise_cfg_scale must not be negative"},
		{"missing expert file", func(c *Config, dir string) {
			c.Defaults.Animate.HighNoiseDiffusionModel = dir + "/nope.gguf"
		}, "defaults.animate.high_noise_diffusion_model"},
		{"expert file unchecked when nobody animates", func(c *Config, dir string) {
			c.Scenes[0].Animate = false
			c.Defaults.Animate.HighNoiseDiffusionModel = dir + "/nope.gguf"
		}, ""},
		{"no expert needs no file", func(c *Config, _ string) {}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := animateConfig(t)
			tc.mutate(cfg, ".")
			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
