package config

import (
	"strings"
	"testing"
)

func pathsCfg() *Config {
	return &Config{
		Project: "demo",
		Output:  Output{File: "out/demo.mp4"},
		Scenes:  []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}},
	}
}

func hasErr(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}

func TestPaths_AbsoluteAndClimbingAreRefused(t *testing.T) {
	cases := []struct {
		name  string
		set   func(*Config)
		field string
		why   string
	}{
		{"absolute output", func(c *Config) { c.Output.File = "/tmp/x.mp4" }, "output.file", "absolute"},
		{"absolute model", func(c *Config) { c.Defaults.Model = "/etc/passwd" }, "defaults.model", "absolute"},
		{"climbing model", func(c *Config) { c.Defaults.Model = "../../models/x.safetensors" }, "defaults.model", `".."`},
		{"climbing in the middle", func(c *Config) { c.Defaults.Voice = "voices/../../x.onnx" }, "defaults.voice", `".."`},
		{"scene clip", func(c *Config) { c.Scenes[0].Clip = "../clip.mp4" }, "scenes[0].clip", `".."`},
		{"scene image", func(c *Config) { c.Scenes[0].Image = "/home/u/a.png" }, "scenes[0].image", "absolute"},
		{"windows drive", func(c *Config) { c.Defaults.Model = `C:\x.gguf` }, "defaults.model", "absolute"},
		{"list of references", func(c *Config) {
			c.Scenes[0].References = &SceneReferences{Images: []string{"sources/a.png", "../b.png"}}
		}, "scenes[0].references.images[1]", `".."`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pathsCfg()
			tc.set(c)
			errs := checkPaths(c)
			if !hasErr(errs, tc.field) || !hasErr(errs, tc.why) {
				t.Fatalf("want an error naming %s and %s, got %v", tc.field, tc.why, errs)
			}
		})
	}
}

func TestPaths_RelativeInsideTheProjectPass(t *testing.T) {
	c := pathsCfg()
	c.Defaults.Model = "models/x.safetensors"
	c.Defaults.Voice = "voices/es.onnx"
	c.Scenes[0].Clip = "sources/intro.mp4"
	c.Scenes[0].Image = "sources/a..b.png" // dots in a name are not a ".." segment
	if errs := checkPaths(c); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestPaths_OutputGoesUnderOut(t *testing.T) {
	for _, tc := range []struct {
		file string
		ok   bool
	}{
		{"out/final.mp4", true}, {"out/sub/final.mp4", true}, {"final.mp4", true}, {"out.mp4", true},
		{"videos/final.mp4", false}, {"../final.mp4", false}, {"/tmp/final.mp4", false}, {"outside/final.mp4", false},
	} {
		c := pathsCfg()
		c.Output.File = tc.file
		errs := checkPaths(c)
		if (len(errs) == 0) != tc.ok {
			t.Errorf("output.file %q: ok=%v, errors %v", tc.file, tc.ok, errs)
		}
	}
	c := pathsCfg()
	c.Output.File = ""
	c.OutputFile = "videos/final.mp4"
	if !hasErr(checkPaths(c), "output_file") {
		t.Error("the deprecated output_file must follow the same rule")
	}
}

func TestPaths_ProjectAndSceneIDsFollowThePattern(t *testing.T) {
	for _, id := range []string{"s06_stand-up.v2", "a", "A1", strings.Repeat("a", 64)} {
		c := pathsCfg()
		c.Scenes[0].ID = id
		if errs := checkPaths(c); len(errs) != 0 {
			t.Errorf("id %q should pass: %v", id, errs)
		}
	}
	for _, id := range []string{"", "../../tmp/x", "a/b", ".hidden", "-x", "a b", strings.Repeat("a", 65), "é"} {
		c := pathsCfg()
		c.Scenes[0].ID = id
		if !hasErr(checkPaths(c), "scene 0") {
			t.Errorf("scene id %q must be refused", id)
		}
	}
	c := pathsCfg()
	c.Project = "../out"
	if !hasErr(checkPaths(c), "project") {
		t.Error("a project name that climbs must be refused")
	}
}

func TestPaths_ValidateReportsThem(t *testing.T) {
	c := pathsCfg()
	c.Scenes[0].ID = "../../x"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "scene 0") {
		t.Fatalf("Validate must report the bad id, got %v", err)
	}
}
