package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func imageProject(t *testing.T) string {
	t.Helper()
	dir := taskProject(t)
	os.WriteFile(filepath.Join(dir, SourcesSubdir, "photo.jpg"), []byte("x"), 0o644)
	return dir
}

func videoCfg(s ...Scene) *Config {
	return &Config{Project: "p", Output: Output{File: "out/f.mp4"}, Scenes: s}
}

func TestSceneImage_Valid(t *testing.T) {
	dir := imageProject(t)
	for _, name := range []string{"photo.jpg", "a.PNG", "b.jpeg", "c.webp"} {
		os.WriteFile(filepath.Join(dir, SourcesSubdir, name), []byte("x"), 0o644)
		if err := videoCfg(Scene{ID: "s", Image: name, TTSText: "hi"}).ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSceneImage_Rejections(t *testing.T) {
	dir := imageProject(t)
	for name, tc := range map[string]struct {
		s    Scene
		want string
	}{
		"missing":   {Scene{ID: "s", Image: "nope.jpg", TTSText: "hi"}, `image "nope.jpg" not found`},
		"path":      {Scene{ID: "s", Image: "../x.jpg", TTSText: "hi"}, "not a path"},
		"audio":     {Scene{ID: "s", Image: "base.mp3", TTSText: "hi"}, "must be the name of a .png"},
		"no prompt": {Scene{ID: "s", TTSText: "hi"}, "empty prompt"},
	} {
		wantErr(t, videoCfg(tc.s).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
	c := videoCfg(Scene{ID: "s", Image: "photo.jpg", TTSText: "hi"})
	c.Output.Mode = ModeImages
	wantErr(t, c.ValidateInDir(linked(dir)), "image only works in video mode")
}

func TestSceneImage_IgnoredFieldsWarn(t *testing.T) {
	dir := imageProject(t)
	two := 2
	c := videoCfg(Scene{ID: "s", Image: "photo.jpg", TTSText: "hi", NegativePrompt: "blur", Lora: "x", Variants: &two})
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"negative_prompt", "lora", "variants"} {
		if !hasWarning(c.Warnings, f+" is ignored because the scene uses its own image") {
			t.Errorf("no warning for %s: %v", f, c.Warnings)
		}
	}
}

func TestSceneImage_AudioFieldsRefusePictures(t *testing.T) {
	dir := imageProject(t)
	wantErr(t, musicCfg(Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "photo.jpg"}).ValidateInDir(linked(dir)), "mp3 or .wav")
	wantErr(t, musicCfg(Track{ID: "a", Prompt: "p", Reference: "photo.jpg"}).ValidateInDir(linked(dir)), "mp3 or .wav")
}

func TestRotationPresetsAreValid(t *testing.T) {
	for _, mt := range []string{MotionRotateCW, MotionRotateCCW} {
		c := videoCfg(Scene{ID: "s", Prompt: "p", TTSText: "hi", Motion: &Motion{Type: mt}})
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", mt, err)
		}
	}
	wantErr(t, videoCfg(Scene{ID: "s", Prompt: "p", TTSText: "hi", Motion: &Motion{Type: "spin"}}).Validate(), "rotate_cw")
}

func TestValidSourceFile(t *testing.T) {
	for n, want := range map[string]bool{"a.mp3": true, "a.wav": true, "a.jpg": true, "a.JPEG": true, "a.png": true, "a.webp": true, "a.gif": false, "a.mp4": true, "a.mkv": true, "a.avi": false, "../a.jpg": false} {
		if got := ValidSourceFile(n); got != want {
			t.Errorf("%q: %v", n, got)
		}
	}
}

func TestInitImage_ValidAndRejections(t *testing.T) {
	dir := imageProject(t)
	half := 0.4
	images := func(s Scene) *Config {
		c := videoCfg(s)
		c.Output = Output{Mode: ModeImages}
		return c
	}
	if err := images(Scene{ID: "s", Prompt: "watercolor", InitImage: "photo.jpg", Strength: &half}).ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	if err := videoCfg(Scene{ID: "s", Prompt: "watercolor", InitImage: "photo.jpg", TTSText: "hi"}).ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("video mode too: %v", err)
	}
	big, neg := 1.5, -0.1
	for name, tc := range map[string]struct {
		s    Scene
		want string
	}{
		"missing":       {Scene{ID: "s", Prompt: "p", InitImage: "nope.png"}, `init_image "nope.png" not found`},
		"path":          {Scene{ID: "s", Prompt: "p", InitImage: "../x.png"}, "not a path"},
		"audio":         {Scene{ID: "s", Prompt: "p", InitImage: "base.mp3"}, "must be the name of a .png"},
		"too strong":    {Scene{ID: "s", Prompt: "p", InitImage: "photo.jpg", Strength: &big}, "strength must be between 0 and 1"},
		"negative":      {Scene{ID: "s", Prompt: "p", InitImage: "photo.jpg", Strength: &neg}, "strength must be between 0 and 1"},
		"strength only": {Scene{ID: "s", Prompt: "p", Strength: &half}, "strength needs an init_image"},
		"no prompt":     {Scene{ID: "s", InitImage: "photo.jpg"}, "empty prompt"},
	} {
		wantErr(t, images(tc.s).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
	wantErr(t, videoCfg(Scene{ID: "s", Prompt: "p", Image: "photo.jpg", InitImage: "photo.jpg", TTSText: "hi"}).ValidateInDir(linked(dir)), "cannot both be set")
}

func TestInitImage_DefaultStrength(t *testing.T) {
	if (Scene{}).ResolvedStrength() != 0.6 {
		t.Fatal("default")
	}
	z := 0.0
	if (Scene{Strength: &z}).ResolvedStrength() != 0 {
		t.Fatal("0 is a value")
	}
}

func TestSceneDescribedByRoundTrips(t *testing.T) {
	cfg := videoCfg(Scene{ID: "s", Prompt: "p", DescribedBy: "gemma4:26b"})
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "described_by: gemma4:26b") {
		t.Fatalf("the model that wrote the prompt must be written to the yaml:\n%s", b)
	}
	var back Config
	if err := yaml.Unmarshal(b, &back); err != nil || back.Scenes[0].DescribedBy != "gemma4:26b" {
		t.Fatalf("described_by must be read back: %v %+v", err, back.Scenes)
	}
}
