package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// legacyImageHash replicates the pre-images-mode seven-field hash so the
// backward-compatibility invariant can be pinned against it.
func legacyImageHash(prompt, negativePrompt string, seed int64, model, resolution, sampler string, steps int) string {
	h := sha256.New()
	fmt.Fprintf(h, "prompt=%s\x00", prompt)
	fmt.Fprintf(h, "negative_prompt=%s\x00", negativePrompt)
	fmt.Fprintf(h, "seed=%d\x00", seed)
	fmt.Fprintf(h, "model=%s\x00", model)
	fmt.Fprintf(h, "resolution=%s\x00", resolution)
	fmt.Fprintf(h, "sampler=%s\x00", sampler)
	fmt.Fprintf(h, "steps=%d\x00", steps)
	return hex.EncodeToString(h.Sum(nil))
}

func baseImageParams() ImageParams {
	return ImageParams{
		Prompt:         "a red fox",
		NegativePrompt: "blurry",
		Seed:           42,
		Model:          "models/sd.safetensors",
		Resolution:     "1024x768",
		Sampler:        "euler",
		Steps:          30,
	}
}

func TestImageHash_LegacyFieldsOnlyMatchesPreChangeDigest(t *testing.T) {
	p := baseImageParams()
	want := legacyImageHash(p.Prompt, p.NegativePrompt, p.Seed, p.Model, p.Resolution, p.Sampler, p.Steps)
	if got := ImageHash(p); got != want {
		t.Errorf("ImageHash with all new fields empty = %s, want legacy digest %s", got, want)
	}
}

func TestImageHash_NewFieldsChangeDigest(t *testing.T) {
	base := ImageHash(baseImageParams())

	cases := []struct {
		name   string
		mutate func(*ImageParams)
	}{
		{"subject_prompt", func(p *ImageParams) { p.SubjectPrompt = "cinematic" }},
		{"global_negative_prompt", func(p *ImageParams) { p.GlobalNegativePrompt = "watermark" }},
		{"lora", func(p *ImageParams) { p.Lora = "<lora:style:0.8>" }},
		{"variant_index", func(p *ImageParams) { p.VariantIndex = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := baseImageParams()
			tc.mutate(&p)
			if got := ImageHash(p); got == base {
				t.Errorf("changing %s did not change the digest", tc.name)
			}
		})
	}
}

func TestImageHash_ChangedSubjectPromptInvalidates(t *testing.T) {
	a := baseImageParams()
	a.SubjectPrompt = "cinematic"
	b := baseImageParams()
	b.SubjectPrompt = "watercolor"
	if ImageHash(a) == ImageHash(b) {
		t.Error("different subject_prompt values must hash differently")
	}
}

func TestImageHash_VariantIndicesHashDifferently(t *testing.T) {
	v0 := baseImageParams()
	v1 := baseImageParams()
	v1.VariantIndex = 1
	if ImageHash(v0) == ImageHash(v1) {
		t.Error("variant_index 0 and 1 must hash differently")
	}
}

func TestVariantKey(t *testing.T) {
	if got := VariantKey("intro", 1, 0); got != "intro" {
		t.Errorf("VariantKey single variant = %q, want %q", got, "intro")
	}
	if got := VariantKey("intro", 3, 0); got != "intro_v1" {
		t.Errorf("VariantKey = %q, want %q", got, "intro_v1")
	}
	if got := VariantKey("intro", 3, 2); got != "intro_v3" {
		t.Errorf("VariantKey = %q, want %q", got, "intro_v3")
	}
}

func TestDecideImages_HitRequiresHashAndPNG(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "scene_intro.png")
	if err := os.WriteFile(png, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{Scenes: map[string]SceneEntry{
		"intro": {ImageHash: "h1", PngPath: png},
	}}

	if !DecideImages(m, "intro", "h1", false) {
		t.Error("matching hash + existing PNG should be a hit")
	}
	if DecideImages(m, "intro", "h2", false) {
		t.Error("hash mismatch should be a miss")
	}
	if DecideImages(m, "missing", "h1", false) {
		t.Error("unknown entry should be a miss")
	}
	if DecideImages(m, "intro", "h1", true) {
		t.Error("--force should always be a miss")
	}

	os.Remove(png)
	if DecideImages(m, "intro", "h1", false) {
		t.Error("missing PNG on disk should be a miss")
	}
}

func TestDecideImages_IndependentOfTTSText(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "scene_intro.png")
	if err := os.WriteFile(png, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The audio hash changes when tts_text changes; images-mode decisions
	// never see it. Simulate two runs with different tts_text (different
	// AudioHash values) and confirm the render decision is identical.
	imageHash := ImageHash(baseImageParams())
	m := &Manifest{Scenes: map[string]SceneEntry{
		"intro": {ImageHash: imageHash, AudioHash: AudioHash(AudioParams{Text: "old narration"}), PngPath: png},
	}}

	if !DecideImages(m, "intro", imageHash, false) {
		t.Fatal("expected hit on first run")
	}

	// Re-run after the narration was reworded: render inputs unchanged.
	m.Scenes["intro"] = SceneEntry{ImageHash: imageHash, AudioHash: AudioHash(AudioParams{Text: "new narration"}), PngPath: png}
	if !DecideImages(m, "intro", imageHash, false) {
		t.Error("changed tts_text must not invalidate the render cache in images mode")
	}
}
