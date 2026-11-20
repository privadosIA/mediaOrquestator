package config

import "testing"

func TestBounds_EachLimitIsEnforced(t *testing.T) {
	big := 65
	hours := 3601.0
	cases := []struct {
		name  string
		set   func(*Config)
		field string
	}{
		{"steps", func(c *Config) { c.Defaults.Steps = MaxSteps + 1 }, "defaults.steps"},
		{"fps", func(c *Config) { c.Defaults.FPS = MaxFPS + 1 }, "defaults.fps"},
		{"animate frames", func(c *Config) { c.Defaults.Animate = &Animate{Frames: MaxFrames + 1} }, "defaults.animate.frames"},
		{"animate steps", func(c *Config) { c.Defaults.Animate = &Animate{Steps: 100000} }, "defaults.animate.steps"},
		{"high noise steps", func(c *Config) { c.Defaults.Animate = &Animate{HighNoiseSteps: MaxSteps + 1} }, "high_noise_steps"},
		{"variants", func(c *Config) { c.Scenes[0].Variants = &big }, "scenes[0].variants"},
		{"silence", func(c *Config) { c.Scenes[0].SilenceSeconds = &hours }, "scenes[0].silence_seconds"},
		{"track duration", func(c *Config) { c.Tracks = []Track{{ID: "a", DurationSeconds: &hours}} }, "tracks[0].duration_seconds"},
		{"inference steps", func(c *Config) { c.Defaults.Music = &Music{InferenceSteps: MaxSteps + 1} }, "inference_steps"},
		{"side", func(c *Config) { c.Defaults.Resolution = "16384x16384" }, "defaults.resolution"},
		{"pixels", func(c *Config) { c.Defaults.Resolution = "4096x4096" }, "defaults.resolution"},
		{"animate resolution", func(c *Config) { c.Defaults.Animate = &Animate{Resolution: "8192x480"} }, "defaults.animate.resolution"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pathsCfg()
			tc.set(c)
			errs := checkBounds(c)
			if !hasErr(errs, tc.field) || !hasErr(errs, "limit") {
				t.Fatalf("want an error naming %s and the limit, got %v", tc.field, errs)
			}
		})
	}
}

func TestBounds_TheLimitItselfPasses(t *testing.T) {
	v := MaxVariants
	s := float64(MaxSilenceSeconds)
	c := pathsCfg()
	c.Defaults.Steps = MaxSteps
	c.Defaults.FPS = MaxFPS
	c.Defaults.Resolution = "3840x2160"
	c.Defaults.Animate = &Animate{Frames: MaxFrames, Steps: MaxSteps, Resolution: "1280x720"}
	c.Scenes[0].Variants = &v
	c.Scenes[0].SilenceSeconds = &s
	if errs := checkBounds(c); len(errs) != 0 {
		t.Fatalf("the limits themselves must pass: %v", errs)
	}
}

// TestBounds_ThePresetsOfThePlannerFitTheLimits: the values the usual presets use stay far below the limits.
func TestBounds_UsualValuesAreFarBelow(t *testing.T) {
	for name, v := range map[string]int{
		"sdxl steps": 30, "turbo steps": 8, "wan steps": 20, "h3 frames": 56, "ltx frames": 121, "wan frames": 81, "fps": 60,
	} {
		if v*2 > MaxFrames || v > MaxSteps*2 {
			t.Errorf("%s = %d is too close to a limit", name, v)
		}
	}
}
