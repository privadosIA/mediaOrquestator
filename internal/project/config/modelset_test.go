package config

import (
	"strings"
	"testing"
)

func modelSetConfig(t *testing.T) (*Config, string) {
	t.Helper()
	dir := t.TempDir()
	one := 1.0
	return &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Defaults: Defaults{
			Resolution: "1024x576",
			Sampler:    "euler",
			Steps:      8,
			ModelSet: &ModelSet{
				DiffusionModel: touchFile(t, dir, "d.safetensors"),
				LLM:            touchFile(t, dir, "llm.gguf"),
				VAE:            touchFile(t, dir, "vae.safetensors"),
				CfgScale:       &one,
			},
		},
		Scenes: []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}},
	}, dir
}

func TestLoad_ModelSetParses(t *testing.T) {
	p := writeTempYAML(t, `project: demo
output_file: out.mp4
defaults:
  model_set:
    diffusion_model: d.gguf
    llm: l.gguf
    vae: v.safetensors
    cfg_scale: 1
scenes: []
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms := cfg.Defaults.ModelSet
	if ms == nil || ms.DiffusionModel != "d.gguf" || ms.LLM != "l.gguf" || ms.VAE != "v.safetensors" || ms.CfgScale == nil || *ms.CfgScale != 1 {
		t.Errorf("model_set = %+v", ms)
	}
}

func TestLoad_AbsentModelSetIsNil(t *testing.T) {
	p := writeTempYAML(t, "project: demo\ndefaults:\n  steps: 4\nscenes: []\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.ModelSet != nil {
		t.Errorf("ModelSet = %+v, want nil", cfg.Defaults.ModelSet)
	}
}

func TestValidate_ModelSet(t *testing.T) {
	neg := -1.0
	cases := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"valid", func(c *Config) {}, ""},
		{"valid without cfg_scale", func(c *Config) { c.Defaults.ModelSet.CfgScale = nil }, ""},
		{"model and model_set together", func(c *Config) { c.Defaults.Model = c.Defaults.ModelSet.LLM }, "defaults.model and defaults.model_set"},
		{"missing vae", func(c *Config) { c.Defaults.ModelSet.VAE = "" }, "defaults.model_set.vae"},
		{"missing llm file", func(c *Config) { c.Defaults.ModelSet.LLM += ".nope" }, "defaults.model_set.llm"},
		{"negative cfg_scale", func(c *Config) { c.Defaults.ModelSet.CfgScale = &neg }, "cfg_scale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := modelSetConfig(t)
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
