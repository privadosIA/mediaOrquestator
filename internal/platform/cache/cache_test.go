package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
)

func TestAudioHash_SameInputsSameHash(t *testing.T) {
	p := AudioParams{Text: "hello", Speed: 1.0, Voice: "v.onnx"}
	first, second := AudioHash(p), AudioHash(p)
	if first != second {
		t.Fatal("AudioHash should be deterministic for identical inputs")
	}
}

func TestAudioHash_DifferentInputsDifferentHash(t *testing.T) {
	a := AudioHash(AudioParams{Text: "hello", Speed: 1.0, Voice: "v.onnx"})
	b := AudioHash(AudioParams{Text: "goodbye", Speed: 1.0, Voice: "v.onnx"})
	if a == b {
		t.Fatal("AudioHash should differ when text differs")
	}
}

func TestImageHash_IndependentOfAudioHash(t *testing.T) {
	img := ImageHash(ImageParams{Prompt: "a cat", Seed: 1, Model: "m", Resolution: "512x512", Sampler: "euler", Steps: 20})
	img2 := ImageHash(ImageParams{Prompt: "a cat", Seed: 1, Model: "m", Resolution: "512x512", Sampler: "euler", Steps: 20})
	if img != img2 {
		t.Fatal("ImageHash should be deterministic for identical inputs")
	}
}

func TestDecide_FullMissWhenNoManifestEntry(t *testing.T) {
	m := &Manifest{Scenes: make(map[string]SceneEntry)}
	d := Decide(m, "s1", "audioHash", "imageHash", "clipHash", false)
	for _, p := range AllPhases {
		if d.Phases[p] {
			t.Fatalf("phase %s should be a miss when no manifest entry exists", p)
		}
	}
}

func TestDecide_PerPhaseIndependence(t *testing.T) {
	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a1", ImageHash: "i1", ClipHash: "c1"},
	}}
	d := Decide(m, "s1", "a1", "i2", "c1", false) // audio matches, image doesn't
	if !d.Phases[PhaseTTS] {
		t.Error("tts should be a hit when audio hash matches")
	}
	if d.Phases[PhaseRender] {
		t.Error("render should be a miss when image hash doesn't match")
	}
	if d.Phases[PhaseClip] {
		t.Error("clip should be a miss when only one of the two hashes matches")
	}
}

func TestDecide_ClipHitRequiresBothHashes(t *testing.T) {
	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a1", ImageHash: "i1", ClipHash: "c1"},
	}}
	d := Decide(m, "s1", "a1", "i1", "c1", false)
	if !d.Phases[PhaseClip] {
		t.Error("clip should be a hit when all three hashes match")
	}
}

func TestDecide_ForceBypassesCache(t *testing.T) {
	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a1", ImageHash: "i1", ClipHash: "c1"},
	}}
	d := Decide(m, "s1", "a1", "i1", "c1", true)
	for _, p := range AllPhases {
		if d.Phases[p] {
			t.Fatalf("phase %s should be a miss under --force", p)
		}
	}
}

func TestDecideWithFilter_NonTargetSceneIsFullyCached(t *testing.T) {
	m := &Manifest{Scenes: make(map[string]SceneEntry)} // empty: would be a full miss without the filter
	f := NewOnlyFilter("target")

	d := DecideWithFilter(m, f, "other-scene", "anyAudio", "anyImage", "anyClip", false)
	for _, p := range AllPhases {
		if !d.Phases[p] {
			t.Fatalf("phase %s should be a hit for a non-target scene under --only", p)
		}
	}
}

func TestDecideWithFilter_TargetSceneUsesNormalDecide(t *testing.T) {
	m := &Manifest{Scenes: make(map[string]SceneEntry)}
	f := NewOnlyFilter("target")

	d := DecideWithFilter(m, f, "target", "anyAudio", "anyImage", "anyClip", false)
	for _, p := range AllPhases {
		if d.Phases[p] {
			t.Fatalf("phase %s should be a miss for the --only target with no manifest entry", p)
		}
	}
}

func TestOnlyFilter_InactiveWhenEmpty(t *testing.T) {
	f := NewOnlyFilter("")
	if f.Active() {
		t.Fatal("empty filter should be inactive")
	}
	if !f.ShouldProcess("anything") {
		t.Fatal("inactive filter should process every scene")
	}
	if f.IsCachedForConcat("anything") {
		t.Fatal("inactive filter should not mark anything as pre-cached")
	}
}

func TestArtifactPaths_StableAcrossCalls(t *testing.T) {
	wav1, png1, mp41 := ArtifactPaths("scene_a")
	wav2, png2, mp42 := ArtifactPaths("scene_a")
	if wav1 != wav2 || png1 != png2 || mp41 != mp42 {
		t.Fatal("ArtifactPaths should return identical paths for the same scene id")
	}
	if wav1 != filepath.Join("tmp", "scene_scene_a.wav") {
		t.Fatalf("wav path = %q, want %q", wav1, filepath.Join("tmp", "scene_scene_a.wav"))
	}
}

func TestManifest_SaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a", ImageHash: "b", WavPath: "w", PngPath: "p", Mp4Path: "m"},
	}}
	if err := SaveManifest(path, m); err != nil {
		t.Fatalf("SaveManifest() error = %v, want nil", err)
	}

	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v, want nil", err)
	}
	if !loaded.Has("s1") {
		t.Fatal("loaded manifest should contain scene s1")
	}
	if loaded.Scenes["s1"].AudioHash != "a" {
		t.Fatalf("AudioHash = %q, want %q", loaded.Scenes["s1"].AudioHash, "a")
	}
}

func TestLoadManifest_MissingFileReturnsEmptyManifest(t *testing.T) {
	m, err := LoadManifest(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v, want nil", err)
	}
	if m.Has("anything") {
		t.Fatal("empty manifest should have no entries")
	}
}

func TestImageHash_ModelSet(t *testing.T) {
	base := ImageParams{Prompt: "p", Seed: 1, Model: "m", Resolution: "512x512", Sampler: "euler", Steps: 8}
	// A params value without a model set hashes exactly as before this field existed.
	h := sha256.New()
	fmt.Fprintf(h, "prompt=p\x00negative_prompt=\x00seed=1\x00model=m\x00resolution=512x512\x00sampler=euler\x00steps=8\x00")
	if got, want := ImageHash(base), hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("hash without model set = %s, want legacy %s", got, want)
	}
	set := base
	set.DiffusionModel, set.LLM, set.VAE, set.CfgScale = "d", "l", "v", 1
	if ImageHash(set) == ImageHash(base) {
		t.Error("model set must change the hash")
	}
	for name, mut := range map[string]func(p *ImageParams){
		"diffusion": func(p *ImageParams) { p.DiffusionModel = "d2" },
		"llm":       func(p *ImageParams) { p.LLM = "l2" },
		"vae":       func(p *ImageParams) { p.VAE = "v2" },
		"cfg":       func(p *ImageParams) { p.CfgScale = 2 },
	} {
		p := set
		mut(&p)
		if ImageHash(p) == ImageHash(set) {
			t.Errorf("changing %s must change the hash", name)
		}
	}
}

func TestImageHash_SourceImageOnlyWhenSet(t *testing.T) {
	base := ImageParams{Prompt: "a", Seed: 1, Model: "m", Resolution: "1280x720", Sampler: "euler", Steps: 30}
	if ImageHash(base) != ImageHash(ImageParams{Prompt: "a", Seed: 1, Model: "m", Resolution: "1280x720", Sampler: "euler", Steps: 30}) {
		t.Fatal("unstable")
	}
	a := ImageHash(ImageParams{Resolution: "1280x720", SourceImage: "abc"})
	if a == ImageHash(ImageParams{Resolution: "1280x720", SourceImage: "abd"}) {
		t.Fatal("the picture content must change the hash")
	}
	if a == ImageHash(ImageParams{Resolution: "640x360", SourceImage: "abc"}) {
		t.Fatal("the resolution it is fitted to must change the hash")
	}
	if ImageHash(base) == ImageHash(ImageParams{Prompt: "a", Seed: 1, Model: "m", Resolution: "1280x720", Sampler: "euler", Steps: 30, SourceImage: "abc"}) {
		t.Fatal("a source image changes the hash")
	}
}

func TestAudioHash_Speaker(t *testing.T) {
	base := AudioParams{Text: "hola", Speed: 1, Voice: "voices/v.onnx"}
	// Speaker 0 hashes as before the field existed, so existing audio stays cached.
	if AudioHash(base) != AudioHash(AudioParams{Text: "hola", Speed: 1, Voice: "voices/v.onnx", Speaker: 0}) {
		t.Error("speaker 0 must not change the hash")
	}
	other := base
	other.Speaker = 1
	if AudioHash(base) == AudioHash(other) {
		t.Error("another speaker must change the hash")
	}
}

func TestAnimateHash_ModelAudioSpeechChangesTheLTXHashOnlyWhenSet(t *testing.T) {
	base := AnimateParams{Engine: "ltx-2.3", DiffusionModel: "m.gguf", ModelAudio: true, Frames: 49}
	speech := base
	speech.ModelAudioSpeech = true
	if AnimateHash(base) == AnimateHash(speech) {
		t.Error("a voice at full level must not reuse a clip mixed as ambience")
	}
	off := base
	off.ModelAudioSpeech = false
	if AnimateHash(base) != AnimateHash(off) {
		t.Error("an unset flag must keep the hash")
	}
}

func TestImageHash_AnEditingSetChangesTheHashOnlyWhenSet(t *testing.T) {
	base := ImageParams{DiffusionModel: "k.gguf", VAE: "ae.safetensors", InitImage: "abc"}
	edit := base
	edit.Edit, edit.EditClipL, edit.EditT5XXL = true, "c.safetensors", "t.safetensors"
	if ImageHash(base) == ImageHash(edit) {
		t.Error("editing a picture must not reuse the img2img result")
	}
	other := edit
	other.EditT5XXL = "t2.safetensors"
	if ImageHash(edit) == ImageHash(other) {
		t.Error("another text encoder must render again")
	}
	same := base
	same.Edit = false
	if ImageHash(base) != ImageHash(same) {
		t.Error("an unset flag must keep the hash")
	}
}

func TestImageHash_ChangingTheEditingEngineRendersAgain(t *testing.T) {
	flux := ImageParams{DiffusionModel: "k.gguf", VAE: "ae.safetensors", InitImage: "abc", Edit: true, EditClipL: "c.safetensors", EditT5XXL: "t.safetensors"}
	qwen := ImageParams{DiffusionModel: "q.gguf", VAE: "qv.safetensors", LLM: "vl.gguf", InitImage: "abc", Edit: true, EditLLMVision: "mm.gguf"}
	if ImageHash(flux) == ImageHash(qwen) {
		t.Error("another engine must render again")
	}
	other := qwen
	other.EditLLMVision = "mm2.gguf"
	if ImageHash(qwen) == ImageHash(other) {
		t.Error("another vision projector must render again")
	}
}

func TestBlenderHash_FollowsEachInputAndNothingElse(t *testing.T) {
	base := BlenderParams{FileSHA256: "f", Inject: `{"Title":"Hi"}`, Samples: 64, Frames: []int{1, 48}, Resolution: "1280x720", FPS: 24, Length: "a", Version: "5.2.3"}
	h := BlenderHash(base)
	if h != BlenderHash(base) {
		t.Fatal("not deterministic")
	}
	for name, mut := range map[string]func(*BlenderParams){
		"file":    func(p *BlenderParams) { p.FileSHA256 = "g" },
		"inject":  func(p *BlenderParams) { p.Inject = `{"Title":"Bye"}` },
		"samples": func(p *BlenderParams) { p.Samples = 8 },
		"frames":  func(p *BlenderParams) { p.Frames = []int{1, 24} },
		"size":    func(p *BlenderParams) { p.Resolution = "640x360" },
		"fps":     func(p *BlenderParams) { p.FPS = 30 },
		"length":  func(p *BlenderParams) { p.Length = "b" },
		"version": func(p *BlenderParams) { p.Version = "5.3.0" },
	} {
		p := base
		mut(&p)
		if BlenderHash(p) == h {
			t.Errorf("%s must change the hash", name)
		}
	}
}
