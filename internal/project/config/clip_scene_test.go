package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clipProject(t *testing.T) string {
	t.Helper()
	dir := taskProject(t)
	os.WriteFile(filepath.Join(dir, SourcesSubdir, "stand-up.mp4"), []byte("x"), 0o644)
	return dir
}

func TestSceneClip_Valid(t *testing.T) {
	dir := clipProject(t)
	for _, name := range []string{"stand-up.mp4", "a.MOV", "b.webm", "c.mkv"} {
		os.WriteFile(filepath.Join(dir, SourcesSubdir, name), []byte("x"), 0o644)
		silence := 1.5
		if err := videoCfg(Scene{ID: "s", Clip: name, SilenceSeconds: &silence}).ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSceneClip_Rejections(t *testing.T) {
	dir := clipProject(t)
	silence := 1.0
	for name, tc := range map[string]struct {
		s    Scene
		want string
	}{
		"missing":   {Scene{ID: "s", Clip: "nope.mp4", SilenceSeconds: &silence}, `clip "nope.mp4" not found`},
		"path":      {Scene{ID: "s", Clip: "../x.mp4", SilenceSeconds: &silence}, "not a path"},
		"not video": {Scene{ID: "s", Clip: "base.mp3", SilenceSeconds: &silence}, "must be the name of a .mp4"},
		"image":     {Scene{ID: "s", Clip: "stand-up.mp4", Image: "photo.jpg", SilenceSeconds: &silence}, "cannot be combined with image"},
		"animate":   {Scene{ID: "s", Clip: "stand-up.mp4", Animate: true, SilenceSeconds: &silence}, "cannot be combined with animate"},
		"motion":    {Scene{ID: "s", Clip: "stand-up.mp4", MotionPrompt: "x", SilenceSeconds: &silence}, "cannot be combined with motion_prompt"},
	} {
		wantErr(t, videoCfg(tc.s).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
	c := videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence})
	c.Output.Mode = ModeImages
	wantErr(t, c.ValidateInDir(linked(dir)), "clip only works in video mode")
}

func TestSceneClip_IgnoredFieldsWarn(t *testing.T) {
	dir := clipProject(t)
	two, silence := 2, 1.0
	c := videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, Prompt: "x", NegativePrompt: "blur", Lora: "x", Variants: &two})
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"prompt", "negative_prompt", "lora", "variants"} {
		if !hasWarning(c.Warnings, f+" is ignored because the scene uses its own clip") {
			t.Errorf("no warning for %s: %v", f, c.Warnings)
		}
	}
}

func TestSceneClip_TrimValidation(t *testing.T) {
	dir := clipProject(t)
	f := func(v float64) *float64 { return &v }
	silence := 1.0
	ok := videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipStart: f(0.5), ClipEnd: f(1.5)})
	if err := ok.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	onlyStart := videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipStart: f(2)})
	if err := onlyStart.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("a start alone is valid (the end is the file's): %v", err)
	}
	for name, tc := range map[string]struct {
		s    Scene
		want string
	}{
		"negative":   {Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipStart: f(-1)}, "clip_start must be >= 0"},
		"end first":  {Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipStart: f(2), ClipEnd: f(1)}, "clip_end must be greater than clip_start"},
		"end only 0": {Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipEnd: f(0)}, "clip_end must be greater"},
		"no clip":    {Scene{ID: "s", Prompt: "p", SilenceSeconds: &silence, ClipStart: f(1)}, "only work with clip or animate"},
	} {
		wantErr(t, videoCfg(tc.s).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
}

func TestSceneClip_TrimAlsoWorksOnAnAnimatedScene(t *testing.T) {
	dir := clipProject(t)
	f := func(v float64) *float64 { return &v }
	c := videoCfg(Scene{ID: "s", Prompt: "p", TTSText: "hi", Animate: true, MotionPrompt: "m", ClipStart: f(0.4), ClipEnd: f(1.6)})
	c.Defaults.Animate = &Animate{DiffusionModel: "w.gguf", T5XXL: "t.gguf", VAE: "v.safetensors"}
	err := c.ValidateInDir(linked(dir))
	if err != nil && (strings.Contains(err.Error(), "clip_start") || strings.Contains(err.Error(), "clip_end")) {
		t.Fatalf("the trim of an animated scene is valid: %v", err)
	}
	wantErr(t, videoCfg(Scene{ID: "s", Prompt: "p", TTSText: "hi", ClipStart: f(1)}).ValidateInDir(linked(dir)), "only work with clip or animate: true")
}

func TestSceneClip_SpeedValidation(t *testing.T) {
	dir := clipProject(t)
	f := func(v float64) *float64 { return &v }
	silence := 1.0
	for _, ok := range []float64{0.25, 0.5, 1, 2, 4} {
		if err := videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipSpeed: f(ok)}).ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%v: %v", ok, err)
		}
	}
	for _, bad := range []float64{0.1, 5, -1} {
		wantErr(t, videoCfg(Scene{ID: "s", Clip: "stand-up.mp4", SilenceSeconds: &silence, ClipSpeed: f(bad)}).ValidateInDir(linked(dir)), "clip_speed must be between")
	}
	wantErr(t, videoCfg(Scene{ID: "s", Prompt: "p", SilenceSeconds: &silence, ClipSpeed: f(1)}).ValidateInDir(linked(dir)), "clip_speed only works with clip or animate")
}

func TestTransitions_Validation(t *testing.T) {
	dir := clipProject(t)
	f := func(v float64) *float64 { return &v }
	ok := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi", TransitionSeconds: f(0)}, Scene{ID: "b", Prompt: "p", TTSText: "hi", TransitionSeconds: f(1.5)})
	ok.Defaults.TransitionSeconds = 0.4
	if err := ok.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	bad := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi", TransitionSeconds: f(3)})
	wantErr(t, bad.ValidateInDir(linked(dir)), "transition_seconds must be between 0 and 2")
	proj := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi"})
	proj.Defaults.TransitionSeconds = -1
	wantErr(t, proj.ValidateInDir(linked(dir)), "defaults.transition_seconds must be between 0 and 2")
	imgs := videoCfg(Scene{ID: "a", Prompt: "p", TransitionSeconds: f(1)})
	imgs.Output.Mode = ModeImages
	wantErr(t, imgs.ValidateInDir(linked(dir)), "transition_seconds only works in video mode")
}

func TestTransitionKinds_ValidationAndResolution(t *testing.T) {
	dir := clipProject(t)
	cfg := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi"}, Scene{ID: "b", Prompt: "p", TTSText: "hi", Transition: "slideleft"})
	cfg.Defaults.Transition = "fadeblack"
	if err := cfg.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	if k := cfg.Scenes[0].ResolvedTransitionKind(cfg.Defaults); k != "fadeblack" {
		t.Errorf("project kind: %s", k)
	}
	if k := cfg.Scenes[1].ResolvedTransitionKind(cfg.Defaults); k != "slideleft" {
		t.Errorf("scene kind: %s", k)
	}
	if k := cfg.Scenes[0].ResolvedTransitionKind(Defaults{}); k != "fade" {
		t.Errorf("default kind: %s", k)
	}
	bad := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi", Transition: "spin"})
	wantErr(t, bad.ValidateInDir(linked(dir)), `transition "spin" is not one of`)
	proj := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi"})
	proj.Defaults.Transition = "nope"
	wantErr(t, proj.ValidateInDir(linked(dir)), "defaults.transition")
	imgs := videoCfg(Scene{ID: "a", Prompt: "p", Transition: "wipeleft"})
	imgs.Output.Mode = ModeImages
	wantErr(t, imgs.ValidateInDir(linked(dir)), "only works in video mode")
}

func TestShaderTransitions_KindAccentAndResolution(t *testing.T) {
	dir := clipProject(t)
	cfg := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi"}, Scene{ID: "b", Prompt: "p", TTSText: "hi", Transition: "shader:glitch", TransitionAccent: "#0af"})
	cfg.Defaults.Transition = "shader:domain-warp"
	cfg.Defaults.TransitionAccent = "#FF6B2B"
	if err := cfg.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	if k := cfg.Scenes[0].ResolvedTransitionKind(cfg.Defaults); k != "shader:domain-warp" {
		t.Errorf("project kind: %s", k)
	}
	if a := cfg.Scenes[0].ResolvedTransitionAccent(cfg.Defaults); a != "#FF6B2B" {
		t.Errorf("project accent: %s", a)
	}
	if a := cfg.Scenes[1].ResolvedTransitionAccent(cfg.Defaults); a != "#0af" {
		t.Errorf("scene accent: %s", a)
	}
	if a := cfg.Scenes[0].ResolvedTransitionAccent(Defaults{}); a != "" {
		t.Errorf("no accent is the shader's own: %q", a)
	}
	for _, bad := range []string{"orange", "#12", "#12345", "ff6b2b", "#gggggg"} {
		c := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi", TransitionAccent: bad})
		wantErr(t, c.ValidateInDir(linked(dir)), "transition_accent")
		d := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi"})
		d.Defaults.TransitionAccent = bad
		wantErr(t, d.ValidateInDir(linked(dir)), "defaults.transition_accent")
	}
	unknown := videoCfg(Scene{ID: "a", Prompt: "p", TTSText: "hi", Transition: "shader:nope"})
	wantErr(t, unknown.ValidateInDir(linked(dir)), "shader:<")
}
