package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editConfig is a valid images project whose model set is an editing one, with its files on disk.
func editConfig(t *testing.T) (*Config, string) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"kontext.gguf", "ae.safetensors", "clip_l.safetensors", "t5.safetensors"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, SourcesSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SourcesSubdir, "foto.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	yaml := `project: edit
output:
  mode: images
defaults:
  voice: kontext.gguf
  resolution: 1024x576
  sampler: euler
  steps: 20
  model_set:
    edit: true
    diffusion_model: kontext.gguf
    vae: ae.safetensors
    clip_l: clip_l.safetensors
    t5xxl: t5.safetensors
    cfg_scale: 1
scenes:
  - id: s1
    prompt: put a red cap on the man
    init_image: foto.png
`
	c, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c, dir
}

func TestModelSetEdit_ValidWithoutAnLLM(t *testing.T) {
	c, dir := editConfig(t)
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("an editing set with its files must validate: %v", err)
	}
}

func TestModelSetEdit_NeedsItsTextEncoders(t *testing.T) {
	c, dir := editConfig(t)
	c.Defaults.ModelSet.T5XXL = ""
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "t5xxl is required") {
		t.Errorf("err = %v", err)
	}
}

func TestModelSetEdit_TheTextEncodersBelongToAnEditingSet(t *testing.T) {
	c, dir := editConfig(t)
	c.Defaults.ModelSet.Edit = false
	c.Defaults.ModelSet.LLM = "clip_l.safetensors"
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "belong to an editing model") {
		t.Errorf("err = %v", err)
	}
}

func TestModelSetEdit_AScenePictureIsNeededAndStrengthIsIgnored(t *testing.T) {
	c, dir := editConfig(t)
	c.Scenes[0].InitImage = ""
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("no picture is a warning, not an error: %v", err)
	}
	if !strings.Contains(strings.Join(c.Warnings, "\n"), "needs the picture to edit") {
		t.Errorf("warnings = %v", c.Warnings)
	}
	c, dir = editConfig(t)
	s := 0.3
	c.Scenes[0].Strength = &s
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(c.Warnings, "\n"), "strength has no effect") {
		t.Errorf("warnings = %v", c.Warnings)
	}
}

// qwenEdit turns an editing config into the Qwen family: an LLM and its vision projector, no CLIP-L or T5-XXL.
func qwenEdit(t *testing.T) (*Config, string) {
	t.Helper()
	c, dir := editConfig(t)
	for _, f := range []string{"qwen.gguf", "mmproj.gguf"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ms := c.Defaults.ModelSet
	ms.ClipL, ms.T5XXL = "", ""
	ms.LLM = "qwen.gguf"
	ms.LLMVision = "mmproj.gguf"
	return c, dir
}

func TestModelSetEdit_QwenFamilyIsValid(t *testing.T) {
	c, dir := qwenEdit(t)
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("a Qwen editing set with llm and llm_vision must validate: %v", err)
	}
	if !c.Defaults.ModelSet.QwenEdit() {
		t.Error("QwenEdit() = false")
	}
}

func TestModelSetEdit_QwenNeedsItsVisionProjector(t *testing.T) {
	c, dir := qwenEdit(t)
	c.Defaults.ModelSet.LLMVision = ""
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "llm_vision is required") {
		t.Errorf("err = %v", err)
	}
}

func TestModelSetEdit_FamiliesAreNotMixed(t *testing.T) {
	c, dir := qwenEdit(t)
	c.Defaults.ModelSet.T5XXL = "t5.safetensors"
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "use one family") {
		t.Errorf("err = %v", err)
	}
}

func TestModelSetEdit_VisionProjectorBelongsToAnEditingSet(t *testing.T) {
	c, dir := qwenEdit(t)
	c.Defaults.ModelSet.Edit = false
	err := c.ValidateInDir(linked(dir))
	if err == nil || !strings.Contains(err.Error(), "belong to an editing model") {
		t.Errorf("err = %v", err)
	}
}
