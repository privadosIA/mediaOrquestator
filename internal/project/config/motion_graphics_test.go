package config

import (
	"errors"
	"strings"
	"testing"
)

// browserChecks makes the browser check answer err.
func browserChecks(err error) Checks { return Checks{Browser: func() error { return err }} }

// videoCfgChecked is videoCfg with the machine answering as k says.
func videoCfgChecked(k Checks, s ...Scene) *Config {
	c := videoCfg(s...)
	c.Checks = k
	return c
}

func TestMotionGraphic_ValidNeedsNoDiffusionFields(t *testing.T) {
	checks := browserChecks(nil)
	dir := clipProject(t)
	c := videoCfgChecked(checks, Scene{ID: "s", MotionGraphic: "three bullets over dark blue", TTSText: "hola"})
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("a motion scene needs no prompt: %v", err)
	}
	silence := 3.0
	if err := videoCfgChecked(checks, Scene{ID: "s", MotionGraphic: "title card", SilenceSeconds: &silence}).ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
}

func TestMotionGraphic_Rejections(t *testing.T) {
	checks := browserChecks(nil)
	dir := clipProject(t)
	silence := 1.0
	for name, tc := range map[string]struct {
		s    Scene
		want string
	}{
		"clip":    {Scene{ID: "s", MotionGraphic: "x", Clip: "stand-up.mp4", SilenceSeconds: &silence}, "motion_graphic cannot be combined with clip"},
		"image":   {Scene{ID: "s", MotionGraphic: "x", Image: "photo.jpg", SilenceSeconds: &silence}, "motion_graphic cannot be combined with image"},
		"animate": {Scene{ID: "s", MotionGraphic: "x", Animate: true, SilenceSeconds: &silence}, "motion_graphic cannot be combined with animate"},
		"render":  {Scene{ID: "s", MotionGraphic: "x", Render: &SceneRender{Steps: 5}, SilenceSeconds: &silence}, "motion_graphic cannot be combined with render"},
		"motionp": {Scene{ID: "s", MotionGraphic: "x", MotionPrompt: "y", SilenceSeconds: &silence}, "motion_graphic cannot be combined with motion_prompt"},
	} {
		wantErr(t, videoCfgChecked(checks, tc.s).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
	c := videoCfgChecked(checks, Scene{ID: "s", MotionGraphic: "x", SilenceSeconds: &silence})
	c.Output.Mode = ModeImages
	wantErr(t, c.ValidateInDir(linked(dir)), "motion_graphic only works in video mode")
}

func TestMotionGraphic_IgnoredFieldsWarn(t *testing.T) {
	checks := browserChecks(nil)
	dir := clipProject(t)
	two, silence := 2, 1.0
	c := videoCfgChecked(checks, Scene{ID: "s", MotionGraphic: "x", SilenceSeconds: &silence, Prompt: "p", NegativePrompt: "n", Lora: "l", Variants: &two})
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"prompt", "negative_prompt", "lora", "variants"} {
		if !hasWarning(c.Warnings, f+" is ignored because the scene is a motion graphic") {
			t.Errorf("no warning for %s: %v", f, c.Warnings)
		}
	}
}

func TestOverlay_ValidOnAnyScene(t *testing.T) {
	checks := browserChecks(nil)
	dir := clipProject(t)
	silence := 4.0
	for _, s := range []Scene{
		{ID: "gen", Prompt: "a lighthouse", TTSText: "hi", Overlay: &Overlay{Text: "Chapter 1"}},
		{ID: "clip", Clip: "stand-up.mp4", SilenceSeconds: &silence, Overlay: &Overlay{Text: "Live", Position: OverlayTop, Start: fp(1), End: fp(3)}},
		{ID: "anim", Prompt: "x", Animate: false, TTSText: "hi", Overlay: &Overlay{Text: "x", Position: OverlayCenter}},
	} {
		if err := videoCfgChecked(checks, s).ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%s: %v", s.ID, err)
		}
	}
}

func TestOverlay_Rejections(t *testing.T) {
	checks := browserChecks(nil)
	dir := clipProject(t)
	silence := 4.0
	for name, tc := range map[string]struct {
		o    *Overlay
		want string
	}{
		"empty text":   {&Overlay{Text: "  "}, "overlay.text is empty"},
		"position":     {&Overlay{Text: "x", Position: "left"}, `overlay.position "left" is invalid`},
		"end<=start":   {&Overlay{Text: "x", Start: fp(2), End: fp(2)}, "overlay.end must be greater than overlay.start"},
		"end only":     {&Overlay{Text: "x", End: fp(0)}, "overlay.end must be greater than overlay.start"},
		"negative":     {&Overlay{Text: "x", Start: fp(-1)}, "overlay.start must be >= 0"},
		"past the end": {&Overlay{Text: "x", Start: fp(1), End: fp(9)}, "overlay.end (9) is beyond the end of the scene, which lasts 4 s"},
		"start beyond": {&Overlay{Text: "x", Start: fp(5)}, "overlay.start (5) is beyond the end of the scene"},
	} {
		wantErr(t, videoCfgChecked(checks, Scene{ID: "s", Prompt: "p", SilenceSeconds: &silence, Overlay: tc.o}).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
	c := videoCfgChecked(checks, Scene{ID: "s", Prompt: "p", TTSText: "hi", Overlay: &Overlay{Text: "x"}})
	c.Output.Mode = ModeImages
	wantErr(t, c.ValidateInDir(linked(dir)), "overlay only works in video mode")
}

func TestMotion_NoBrowserNamesTheScenesAndTheFix(t *testing.T) {
	checks := browserChecks(errors.New("no Chrome or Chromium found: install Google Chrome or Chromium with your system package manager, or set its path in Settings"))
	dir := clipProject(t)
	silence := 2.0
	c := videoCfgChecked(checks,
		Scene{ID: "plain", Prompt: "p", SilenceSeconds: &silence},
		Scene{ID: "title", Prompt: "p", SilenceSeconds: &silence, Overlay: &Overlay{Text: "x"}},
		Scene{ID: "gfx", MotionGraphic: "chart", SilenceSeconds: &silence},
	)
	err := c.ValidateInDir(linked(dir))
	if err == nil {
		t.Fatal("scenes that need a browser must fail the validation when there is none")
	}
	for _, want := range []string{`"title"`, `"gfx"`, "Settings"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message must contain %s: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), `"plain"`) {
		t.Errorf("a scene without motion must not be named: %v", err)
	}
}

func TestMotion_NoBrowserDoesNotAffectProjectsWithoutMotion(t *testing.T) {
	checks := browserChecks(errors.New("no browser"))
	dir := clipProject(t)
	silence := 2.0
	if err := videoCfgChecked(checks, Scene{ID: "plain", Prompt: "p", SilenceSeconds: &silence}).ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("no motion, no browser needed: %v", err)
	}
}
