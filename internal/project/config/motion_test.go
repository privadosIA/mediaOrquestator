package config

import (
	"strings"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestMotionLook_ParsingDistinguishesAbsentFromExplicit(t *testing.T) {
	yamlContent := `
project: p
output:
  mode: video
  file: out.mp4
scenes:
  - id: plain
    prompt: a
    tts_text: t
  - id: moving
    prompt: a
    tts_text: t
    motion:
      type: zoom_in
      intensity: 0.0
    look:
      grain: 0.3
      white_balance: warm
  - id: quiet
    prompt: a
    silence_seconds: 1.5
`
	cfg, err := Load(writeTempYAML(t, yamlContent))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	plain, moving, quiet := cfg.Scenes[0], cfg.Scenes[1], cfg.Scenes[2]

	if plain.Motion != nil || plain.Look != nil || plain.SilenceSeconds != nil {
		t.Error("absent fields must parse as nil")
	}
	if plain.MotionType() != MotionStatic {
		t.Errorf("MotionType() = %q, want static default", plain.MotionType())
	}
	if plain.MotionIntensity() != DefaultMotionIntensity {
		t.Errorf("MotionIntensity() = %v, want default %v", plain.MotionIntensity(), DefaultMotionIntensity)
	}
	if plain.LookGrain() != 0.0 || plain.LookWhiteBalance() != WBNeutral {
		t.Error("absent look must resolve to grain 0.0 / neutral")
	}
	if plain.IsSilent() || plain.SilenceDuration() != 0 {
		t.Error("absent silence_seconds must resolve to narrated scene")
	}

	if moving.MotionType() != MotionZoomIn {
		t.Errorf("MotionType() = %q", moving.MotionType())
	}
	if moving.Motion.Intensity == nil || *moving.Motion.Intensity != 0.0 {
		t.Error("explicit intensity 0.0 must be distinguishable from absent")
	}
	if moving.MotionIntensity() != 0.0 {
		t.Errorf("MotionIntensity() = %v, want explicit 0.0", moving.MotionIntensity())
	}
	if moving.LookGrain() != 0.3 || moving.LookWhiteBalance() != WBWarm {
		t.Errorf("look = %v/%q", moving.LookGrain(), moving.LookWhiteBalance())
	}

	if !quiet.IsSilent() || quiet.SilenceDuration() != 1.5 {
		t.Errorf("quiet scene: IsSilent=%v dur=%v", quiet.IsSilent(), quiet.SilenceDuration())
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_MotionLookSilence(t *testing.T) {
	base := func(mutate func(*Scene)) *Config {
		s := Scene{ID: "s1", Prompt: "a", TTSText: "t"}
		mutate(&s)
		return &Config{
			Project: "p",
			Output:  Output{Mode: ModeVideo, File: "out.mp4"},
			Scenes:  []Scene{s},
		}
	}

	cases := []struct {
		name    string
		cfg     *Config
		wantErr string // substring; empty means accepted
	}{
		{
			name:    "invalid motion type rejected",
			cfg:     base(func(s *Scene) { s.Motion = &Motion{Type: "dolly"} }),
			wantErr: "motion.type",
		},
		{
			name:    "intensity above range rejected",
			cfg:     base(func(s *Scene) { s.Motion = &Motion{Type: MotionZoomIn, Intensity: floatPtr(1.5)} }),
			wantErr: "motion.intensity must be within [0.0, 1.0]",
		},
		{
			name:    "negative intensity rejected",
			cfg:     base(func(s *Scene) { s.Motion = &Motion{Type: MotionPanLeft, Intensity: floatPtr(-0.1)} }),
			wantErr: "motion.intensity",
		},
		{
			name:    "grain above range rejected",
			cfg:     base(func(s *Scene) { s.Look = &Look{Grain: 1.2} }),
			wantErr: "look.grain",
		},
		{
			name:    "invalid white balance rejected",
			cfg:     base(func(s *Scene) { s.Look = &Look{WhiteBalance: "sepia"} }),
			wantErr: "look.white_balance",
		},
		{
			name:    "zero silence_seconds rejected",
			cfg:     base(func(s *Scene) { s.TTSText = ""; s.SilenceSeconds = floatPtr(0) }),
			wantErr: "silence_seconds must be > 0",
		},
		{
			name:    "silence with narration rejected",
			cfg:     base(func(s *Scene) { s.SilenceSeconds = floatPtr(2.0) }),
			wantErr: "mutually exclusive",
		},
		{
			name:    "silent scene without tts_text accepted in video mode",
			cfg:     base(func(s *Scene) { s.TTSText = ""; s.SilenceSeconds = floatPtr(2.0) }),
			wantErr: "",
		},
		{
			name: "valid motion and look accepted",
			cfg: base(func(s *Scene) {
				s.Motion = &Motion{Type: MotionHandheld, Intensity: floatPtr(0.8)}
				s.Look = &Look{Grain: 0.4, WhiteBalance: WBCool}
			}),
			wantErr: "",
		},
		{
			name: "empty tts_text without silence still rejected",
			cfg:  base(func(s *Scene) { s.TTSText = "" }),
			// unchanged video-mode rule
			wantErr: "empty tts_text",
		},
		{
			name: "invalid motion rejected in images mode too",
			cfg: &Config{
				Project: "p",
				Output:  Output{Mode: ModeImages},
				Scenes:  []Scene{{ID: "s1", Prompt: "a", Motion: &Motion{Type: "dolly"}}},
			},
			wantErr: "motion.type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidate_MotionLookSilenceAreIgnoredInSilenceInImagesMode(t *testing.T) {
	cfg := &Config{
		Project: "p",
		Output:  Output{Mode: ModeImages},
		Scenes: []Scene{{
			ID: "s1", Prompt: "a",
			Motion:         &Motion{Type: MotionZoomIn},
			Look:           &Look{Grain: 0.5},
			SilenceSeconds: floatPtr(2.0),
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected acceptance with warnings, got: %v", err)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("no warnings expected, got %v", cfg.Warnings)
	}
}
