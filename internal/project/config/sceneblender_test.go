package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// blenderProject is a video project with one Blender scene and its file in the sources folder; Blender is available.
func blenderProject(t *testing.T) (*Config, string) {
	t.Helper()
	dir := taskProject(t)
	os.WriteFile(filepath.Join(dir, SourcesSubdir, "turn.blend"), []byte("BLENDER"), 0o644)
	c := &Config{
		Project:  "demo",
		Output:   Output{File: "out.mp4"},
		Defaults: Defaults{Resolution: "1280x720", FPS: 24},
		Scenes:   []Scene{{ID: "s1", TTSText: "hola", Blender: &SceneBlender{File: "turn.blend", Inject: map[string]any{"Title": "Hello", "Mat": []any{1.0, 0.2, 0.2}}}}},
		Checks:   Checks{Blender: func() error { return nil }},
	}
	return c, dir
}

func TestSceneBlender_ValidNeedsNoPromptOrModel(t *testing.T) {
	c, dir := blenderProject(t)
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("a valid Blender scene: %v", err)
	}
	if !c.Scenes[0].IsBlender() {
		t.Error("IsBlender")
	}
	b := c.Scenes[0].Blender
	if b.ResolvedDevice() != BlenderAuto || b.ResolvedSamples() != DefaultBlenderSamples {
		t.Errorf("defaults: %s %d", b.ResolvedDevice(), b.ResolvedSamples())
	}
}

func TestSceneBlender_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		edit func(*Scene)
		want string
	}{
		"no file":        {func(s *Scene) { s.Blender.File = "" }, "blender.file is required"},
		"a path":         {func(s *Scene) { s.Blender.File = "../turn.blend" }, "not a path"},
		"not .blend":     {func(s *Scene) { s.Blender.File = "turn.png" }, "must be the name of a .blend file"},
		"missing file":   {func(s *Scene) { s.Blender.File = "gone.blend" }, "gone.blend\" not found"},
		"bad device":     {func(s *Scene) { s.Blender.Device = "igpu" }, "blender.device"},
		"samples":        {func(s *Scene) { s.Blender.Samples = 99999 }, "blender.samples"},
		"frames shape":   {func(s *Scene) { s.Blender.Frames = []int{1} }, "blender.frames"},
		"frames order":   {func(s *Scene) { s.Blender.Frames = []int{9, 3} }, "blender.frames"},
		"inject value":   {func(s *Scene) { s.Blender.Inject = map[string]any{"x": map[string]any{"a": 1}} }, "blender.inject"},
		"inject list":    {func(s *Scene) { s.Blender.Inject = map[string]any{"x": []any{"a"}} }, "list may only hold numbers"},
		"with animate":   {func(s *Scene) { s.Animate = true }, "cannot be combined with animate"},
		"with an image":  {func(s *Scene) { s.Image = "a.png" }, "cannot be combined with image"},
		"with a graphic": {func(s *Scene) { s.MotionGraphic = "x" }, "cannot be combined with motion_graphic"},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := blenderProject(t)
			tc.edit(&c.Scenes[0])
			err := c.ValidateInDir(linked(dir))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSceneBlender_OnlyInVideoMode(t *testing.T) {
	c, dir := blenderProject(t)
	c.Output.Mode = ModeImages
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "blender only works in video mode") {
		t.Errorf("err = %v", err)
	}
}

func TestSceneBlender_PromptIsIgnoredWithAWarning(t *testing.T) {
	c, dir := blenderProject(t)
	c.Scenes[0].Prompt = "a cat"
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range c.Warnings {
		if strings.Contains(w, "prompt is ignored") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning: %v", c.Warnings)
	}
}

func TestSceneBlender_RefusedWhenBlenderIsUnavailable(t *testing.T) {
	c, dir := blenderProject(t)
	c.Checks.Blender = func() error { return errors.New("Blender is not installed: install Blender 4.2 or newer") }
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), `scenes "s1" are rendered by Blender`) || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("err = %v", err)
	}
	// the import writes projects that run elsewhere
	c.SkipBlenderCheck = true
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("skipped: %v", err)
	}
}

func TestSceneBlender_UnavailableDoesNotAffectOtherProjects(t *testing.T) {
	c, dir := blenderProject(t)
	c.Scenes = []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}}
	c.Checks.Blender = func() error { return errors.New("never asked") }
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("a project without Blender scenes: %v", err)
	}
}

func TestSceneBlender_YAMLRoundTrip(t *testing.T) {
	const doc = `
id: s1
tts_text: hola
blender:
  file: turn.blend
  samples: 32
  device: gpu
  frames: [1, 48]
  inject:
    Title: "He said \"hi\""
    Floor: [0.1, 0.2, 0.3]
    Cube.speed: 2
`
	var s Scene
	if err := yaml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatal(err)
	}
	b := s.Blender
	if b == nil || b.File != "turn.blend" || b.Samples != 32 || b.Device != "gpu" || len(b.Frames) != 2 || b.Inject["Title"] != `He said "hi"` || b.Inject["Cube.speed"] != 2 {
		t.Fatalf("blender = %+v", b)
	}
	if err := checkInjectValue(b.Inject["Floor"]); err != nil {
		t.Errorf("a colour is a list of numbers: %v", err)
	}
}

func TestValidSourceFileAcceptsBlend(t *testing.T) {
	if !ValidSourceFile("scene.blend") || ValidSourceFile("../scene.blend") || ValidSourceFile("scene.blend.exe") {
		t.Error("ValidSourceFile and .blend")
	}
}
