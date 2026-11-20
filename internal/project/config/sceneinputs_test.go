package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// inputsProject is a project folder with the files of every kind a scene can name, and the config of an animated
// scene with a picture of its own on the engine given.
func inputsProject(t *testing.T, mk func(*testing.T) *Config) (*Config, string) {
	t.Helper()
	dir := taskProject(t)
	for _, n := range []string{"a.png", "b.png", "end.png", "clip.mp4", "voice.wav", "voice.m4a", "doc.txt"} {
		os.WriteFile(filepath.Join(dir, SourcesSubdir, n), []byte("x"), 0o644)
	}
	c := mk(t)
	c.Scenes[0].Image = "a.png"
	return c, dir
}

func firstFrameConfig(t *testing.T) *Config {
	c := miniMaxConfig(t)
	c.Defaults.Animate.FirstFrame = true
	return c
}

func TestSceneInputs_ValidCases(t *testing.T) {
	c, dir := inputsProject(t, firstFrameConfig)
	c.Scenes[0].EndImage = "end.png"
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("H3 first/last frame: %v", err)
	}
	c, dir = inputsProject(t, ltxConfig)
	c.Scenes[0].EndImage = "end.png"
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("LTX last picture: %v", err)
	}
	c, dir = inputsProject(t, miniMaxConfig)
	c.Scenes[0].References = &SceneReferences{Images: []string{"b.png"}, Videos: []string{"clip.mp4"}, Audios: []string{"voice.wav", "voice.m4a"}}
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("H3 references: %v", err)
	}
}

func TestSceneInputs_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		mk   func(*testing.T) *Config
		mut  func(*Config)
		want string
	}{
		"last picture on wan":        {animateConfig, func(c *Config) { c.Scenes[0].EndImage = "end.png" }, "end_image needs minimax-h3 with first_frame or ltx-2.3"},
		"last picture on references": {miniMaxConfig, func(c *Config) { c.Scenes[0].EndImage = "end.png" }, "end_image needs minimax-h3 with first_frame"},
		"references on ltx":          {ltxConfig, func(c *Config) { c.Scenes[0].References = &SceneReferences{Images: []string{"b.png"}} }, "references need minimax-h3 without first_frame"},
		"references on first frame":  {firstFrameConfig, func(c *Config) { c.Scenes[0].References = &SceneReferences{Images: []string{"b.png"}} }, "references need minimax-h3 without first_frame"},
		"both": {miniMaxConfig, func(c *Config) {
			c.Scenes[0].EndImage = "end.png"
			c.Scenes[0].References = &SceneReferences{Images: []string{"b.png"}}
		}, "cannot be combined"},
		"no scene picture":  {firstFrameConfig, func(c *Config) { c.Scenes[0].Image = ""; c.Scenes[0].EndImage = "end.png" }, "need the scene's own picture"},
		"not animated":      {firstFrameConfig, func(c *Config) { c.Scenes[0].Animate = false; c.Scenes[0].EndImage = "end.png" }, "animated scene"},
		"missing picture":   {firstFrameConfig, func(c *Config) { c.Scenes[0].EndImage = "nope.png" }, `end_image "nope.png" not found`},
		"a path":            {firstFrameConfig, func(c *Config) { c.Scenes[0].EndImage = "../end.png" }, "not a path"},
		"picture is a clip": {firstFrameConfig, func(c *Config) { c.Scenes[0].EndImage = "clip.mp4" }, "must be the name of a .png"},
		"reference picture missing": {miniMaxConfig, func(c *Config) {
			c.Scenes[0].References = &SceneReferences{Images: []string{"nope.png"}}
		}, `references.images "nope.png" not found`},
		"reference video is a picture": {miniMaxConfig, func(c *Config) {
			c.Scenes[0].References = &SceneReferences{Videos: []string{"a.png"}}
		}, "references.videos"},
		"reference audio is text": {miniMaxConfig, func(c *Config) {
			c.Scenes[0].References = &SceneReferences{Audios: []string{"doc.txt"}}
		}, "references.audios"},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := inputsProject(t, tc.mk)
			tc.mut(c)
			wantErr(t, c.ValidateInDir(linked(dir)), tc.want)
		})
	}
}

func TestSceneInputs_ASceneWithNoneValidatesAsBefore(t *testing.T) {
	for _, mk := range []func(*testing.T) *Config{animateConfig, miniMaxConfig, ltxConfig, firstFrameConfig} {
		c, dir := inputsProject(t, mk)
		c.Scenes[0].References = &SceneReferences{} // an empty block is no reference
		if err := c.ValidateInDir(linked(dir)); err != nil {
			t.Errorf("a scene with its picture alone: %v", err)
		}
	}
}

func TestSceneInputs_YAMLRoundTrip(t *testing.T) {
	in := Scene{ID: "s", Prompt: "p", TTSText: "t", Animate: true, Image: "a.png",
		References: &SceneReferences{Images: []string{"b.png"}, Videos: []string{"clip.mp4"}, Audios: []string{"voice.wav"}}}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var back Scene
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.References == nil || back.References.Images[0] != "b.png" || back.References.Videos[0] != "clip.mp4" || back.References.Audios[0] != "voice.wav" {
		t.Errorf("round trip: %s -> %+v", out, back.References)
	}
	end, _ := yaml.Marshal(Scene{ID: "s", Image: "a.png", EndImage: "end.png"})
	var b2 Scene
	yaml.Unmarshal(end, &b2)
	if b2.EndImage != "end.png" {
		t.Errorf("end_image: %s", end)
	}
	plain, _ := yaml.Marshal(Scene{ID: "s", Image: "a.png"})
	if s := string(plain); containsAny(s, "end_image", "references") {
		t.Errorf("a scene with none writes none: %s", s)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		for i := 0; i+len(x) <= len(s); i++ {
			if s[i:i+len(x)] == x {
				return true
			}
		}
	}
	return false
}
