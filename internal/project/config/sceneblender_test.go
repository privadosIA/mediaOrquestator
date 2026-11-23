package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestSceneBlender_ABuiltSceneRendersAt16SamplesAndAFileSceneAt64(t *testing.T) {
	if got := (SceneBlender{Build: "t.scene.json"}).ResolvedSamples(); got != 16 {
		t.Errorf("a built scene without samples = %d, want 16", got)
	}
	if got := (SceneBlender{File: "turn.blend"}).ResolvedSamples(); got != 64 {
		t.Errorf("a .blend scene without samples = %d, want 64", got)
	}
	if got := (SceneBlender{Build: "t.scene.json", Samples: 40}).ResolvedSamples(); got != 40 {
		t.Errorf("the samples a scene gives are used: %d", got)
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

// buildProject is blenderProject with a description in the sources and a scene that builds it.
func buildProject(t *testing.T) (*Config, string) {
	t.Helper()
	c, dir := blenderProject(t)
	os.WriteFile(filepath.Join(dir, SourcesSubdir, "titulo.scene.json"), []byte(`{"version":1}`), 0o644)
	c.Scenes[0].Blender = &SceneBlender{Build: "titulo.scene.json", Samples: 32}
	return c, dir
}

func TestSceneBlender_ABuiltSceneIsValidAndNeedsNoModel(t *testing.T) {
	c, dir := buildProject(t)
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("a valid built scene: %v", err)
	}
	if !c.Scenes[0].IsBlender() || !c.Scenes[0].Blender.IsBuild() {
		t.Error("IsBlender / IsBuild")
	}
}

func TestSceneBlender_BuildRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		edit func(*Scene)
		want string
	}{
		"neither":        {func(s *Scene) { s.Blender.Build = "" }, "blender.file is required"},
		"both":           {func(s *Scene) { s.Blender.File = "turn.blend" }, "cannot both be given"},
		"with inject":    {func(s *Scene) { s.Blender.Inject = map[string]any{"Title": "x"} }, "blender.inject cannot be used with blender.build"},
		"with frames":    {func(s *Scene) { s.Blender.Frames = []int{1, 10} }, "blender.frames cannot be used with blender.build"},
		"not a spec":     {func(s *Scene) { s.Blender.Build = "titulo.json" }, "must be the name of a .scene.json file"},
		"a path":         {func(s *Scene) { s.Blender.Build = "sub/titulo.scene.json" }, "not a path"},
		"parent folder":  {func(s *Scene) { s.Blender.Build = "../x.scene.json" }, "not a path"},
		"absolute":       {func(s *Scene) { s.Blender.Build = "/etc/x.scene.json" }, "not a path"},
		"missing":        {func(s *Scene) { s.Blender.Build = "gone.scene.json" }, `gone.scene.json" not found`},
		"with animate":   {func(s *Scene) { s.Animate = true }, "cannot be combined with animate"},
		"with a graphic": {func(s *Scene) { s.MotionGraphic = "x" }, "cannot be combined with motion_graphic"},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := buildProject(t)
			tc.edit(&c.Scenes[0])
			err := c.ValidateInDir(linked(dir))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSceneBlender_ASymlinkOutOfTheSourcesIsRefused(t *testing.T) {
	for name, edit := range map[string]func(*Scene){
		"build": func(s *Scene) { s.Blender = &SceneBlender{Build: "out.scene.json"} },
		"file":  func(s *Scene) { s.Blender = &SceneBlender{File: "out.blend"} },
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := buildProject(t)
			outside := t.TempDir()
			target := filepath.Join(outside, "secret")
			os.WriteFile(target, []byte("x"), 0o644)
			for _, n := range []string{"out.scene.json", "out.blend"} {
				if err := os.Symlink(target, filepath.Join(dir, SourcesSubdir, n)); err != nil {
					t.Fatal(err)
				}
			}
			edit(&c.Scenes[0])
			err := c.ValidateInDir(linked(dir))
			if err == nil || !strings.Contains(err.Error(), "resolves outside") {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestSceneBlender_ASymlinkInsideTheSourcesIsFine(t *testing.T) {
	c, dir := buildProject(t)
	if err := os.Symlink("titulo.scene.json", filepath.Join(dir, SourcesSubdir, "alias.scene.json")); err != nil {
		t.Fatal(err)
	}
	c.Scenes[0].Blender.Build = "alias.scene.json"
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Errorf("a link inside the sources: %v", err)
	}
}

func TestSceneBlender_BuildYAMLRoundTrip(t *testing.T) {
	var s Scene
	if err := yaml.Unmarshal([]byte("id: s1\nblender:\n  build: titulo.scene.json\n  samples: 32\n"), &s); err != nil {
		t.Fatal(err)
	}
	if s.Blender == nil || s.Blender.Build != "titulo.scene.json" || s.Blender.File != "" || s.Blender.Samples != 32 {
		t.Fatalf("parsed: %+v", s.Blender)
	}
	out, _ := yaml.Marshal(s)
	if strings.Contains(string(out), "file:") {
		t.Errorf("an empty file is written: %s", out)
	}
}

func TestSourceNames_SceneDescriptionsAndFonts(t *testing.T) {
	for name, want := range map[string]bool{"a.scene.json": true, "A.SCENE.JSON": true, ".scene.json": false, "a.json": false, "a/b.scene.json": false} {
		if got := ValidSceneSpecName(name); got != want {
			t.Errorf("ValidSceneSpecName(%q) = %v", name, got)
		}
	}
	for name, want := range map[string]bool{"a.ttf": true, "B.OTF": true, "a.woff2": false, "../a.ttf": false} {
		if got := ValidFontName(name); got != want {
			t.Errorf("ValidFontName(%q) = %v", name, got)
		}
	}
	if !ValidSourceFile("a.scene.json") || !ValidSourceFile("a.ttf") {
		t.Error("descriptions and fonts can be stored in the sources")
	}
}

func TestSceneBlender_TheExampleProjectIsValid(t *testing.T) {
	_, here, _, _ := runtime.Caller(0) // other tests change the working directory
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "testdata", "blender-build")
	c, err := Load(filepath.Join(dir, "scenes.yaml"))
	if err != nil {
		t.Fatalf("the example project: %v", err)
	}
	c.Checks = Checks{Blender: func() error { return nil }}
	if err := c.ValidateInDir(dir); err != nil {
		t.Fatalf("the example project: %v", err)
	}
	s := c.Scenes[0]
	if !s.IsBlender() || !s.Blender.IsBuild() || s.Blender.Build != "titulo.scene.json" || s.SilenceDuration() != 5 || c.Defaults.FPS != 30 {
		t.Errorf("scene = %+v", s)
	}
}
