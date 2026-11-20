package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestMain runs the tests inside a scratch working directory, so fixtures can use relative paths.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "config-tests")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "scenes-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// relDir returns a directory under the test's working directory (see TestMain) that stands for dir, as a
// relative path: paths in a scenes.yaml must be relative to the project, so the fixtures use relative ones.
func relDir(dir string) string {
	r := "d-" + filepath.Base(filepath.Dir(dir)) + "-" + filepath.Base(dir)
	if err := os.MkdirAll(r, 0o755); err != nil {
		panic(err)
	}
	return r
}

// linked makes the fixture folders the tests created under the working directory (d-*, music-models*) reachable
// from dir and returns dir: ValidateInDir resolves relative paths against the project folder it is given.
func linked(dir string) string {
	wd, _ := os.Getwd()
	ents, _ := os.ReadDir(".")
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "d-") && !strings.HasPrefix(e.Name(), "music-models") {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, e.Name())); err != nil {
			_ = os.Symlink(filepath.Join(wd, e.Name()), filepath.Join(dir, e.Name()))
		}
	}
	return dir
}

// touchFile creates a stub file named name in the directory that stands for dir and returns its relative path.
func touchFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := relDir(dir) + "/" + name
	if err := os.WriteFile(p, []byte("stub"), 0o644); err != nil {
		t.Fatalf("touch %s: %v", p, err)
	}
	return p
}

func intPtr(v int) *int { return &v }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestValidate_ValidConfig_ReturnsNoError(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "model.safetensors")
	voice := touchFile(t, dir, "voice.wav")

	cfg := &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Defaults: Defaults{
			Model:      model,
			Voice:      voice,
			Resolution: "1280x720",
			Sampler:    "euler",
			Steps:      30,
			FPS:        24,
		},
		Scenes: []Scene{
			{ID: "s1", Prompt: "a cat", TTSText: "hello", Seed: intPtr(42), Speed: 1.0},
			{ID: "s2", Prompt: "a dog", TTSText: "world", Seed: nil, Speed: 0.8},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingOutputFile_ReturnsError(t *testing.T) {
	cfg := &Config{
		Project: "demo",
		Scenes:  []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing output_file, got nil")
	}
	if !contains(err.Error(), "output_file") {
		t.Errorf("error should mention output_file, got: %v", err)
	}
}

func TestValidate_OutputFileNeedNotExistOnDisk(t *testing.T) {
	cfg := &Config{
		Project:    "demo",
		OutputFile: "out/nonexistent/path/out.mp4",
		Scenes:     []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("output_file should not need to exist yet, got error: %v", err)
	}
}

func TestValidate_MissingModelFile_ReturnsError(t *testing.T) {
	cfg := &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Defaults:   Defaults{Model: "/nonexistent/model.safetensors"},
		Scenes:     []Scene{{ID: "s1", Prompt: "p", TTSText: "t"}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model file, got nil")
	}
	if !contains(err.Error(), "defaults.model") {
		t.Errorf("error should mention defaults.model, got: %v", err)
	}
}

func TestValidate_DuplicateSceneID_ReturnsError(t *testing.T) {
	cfg := &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Scenes: []Scene{
			{ID: "hero", Prompt: "p1", TTSText: "t1"},
			{ID: "hero", Prompt: "p2", TTSText: "t2"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate scene id, got nil")
	}
	if !contains(err.Error(), "duplicate scene id") {
		t.Errorf("error should mention duplicate scene id, got: %v", err)
	}
	if !contains(err.Error(), "hero") {
		t.Errorf("error should name the offending id 'hero', got: %v", err)
	}
}

func TestValidate_EmptyPrompt_ReturnsError(t *testing.T) {
	cfg := &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Scenes: []Scene{
			{ID: "good", Prompt: "ok", TTSText: "ok"},
			{ID: "bad", Prompt: "", TTSText: "ok"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
	if !contains(err.Error(), "bad") {
		t.Errorf("error should name scene id 'bad', got: %v", err)
	}
	if !contains(err.Error(), "empty prompt") {
		t.Errorf("error should mention empty prompt, got: %v", err)
	}
}

func TestValidate_EmptyTTSText_ReturnsError(t *testing.T) {
	cfg := &Config{
		Project:    "demo",
		OutputFile: "out.mp4",
		Scenes: []Scene{
			{ID: "good", Prompt: "ok", TTSText: "ok"},
			{ID: "bad", Prompt: "ok", TTSText: ""},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty tts_text, got nil")
	}
	if !contains(err.Error(), "bad") {
		t.Errorf("error should name scene id 'bad', got: %v", err)
	}
	if !contains(err.Error(), "empty tts_text") {
		t.Errorf("error should mention empty tts_text, got: %v", err)
	}
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	_, err := Load("/nonexistent/scenes.yaml")
	if err == nil {
		t.Fatal("expected error loading missing file, got nil")
	}
	if !contains(err.Error(), "cannot read") {
		t.Errorf("error should mention read failure, got: %v", err)
	}
}

func TestLoad_ValidYAML_ParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "model.safetensors")
	voice := touchFile(t, dir, "voice.wav")

	yamlContent := fmt.Sprintf(`
project: demo
output_file: out.mp4
defaults:
  model: %s
  voice: %s
  resolution: 1920x1080
  sampler: dpmpp_2m
  steps: 50
  fps: 30
scenes:
  - id: intro
    prompt: a wide shot of a city
    negative_prompt: blurry
    seed: 0
    tts_text: Welcome to the city
    speed: 1.0
  - id: outro
    prompt: a sunset
    negative_prompt: dark
    tts_text: Goodbye
    speed: 0.5
`, model, voice)

	path := writeTempYAML(t, yamlContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project != "demo" {
		t.Errorf("Project = %q, want %q", cfg.Project, "demo")
	}
	if len(cfg.Scenes) != 2 {
		t.Fatalf("len(Scenes) = %d, want 2", len(cfg.Scenes))
	}
	if cfg.Scenes[0].Seed == nil {
		t.Error("Scenes[0].Seed should be non-nil (explicit 0)")
	} else if *cfg.Scenes[0].Seed != 0 {
		t.Errorf("Scenes[0].Seed = %d, want 0", *cfg.Scenes[0].Seed)
	}
	if cfg.Scenes[1].Seed != nil {
		t.Errorf("Scenes[1].Seed should be nil (absent), got %d", *cfg.Scenes[1].Seed)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLoadBytes_MatchesFileLoader(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "model.safetensors")
	voice := touchFile(t, dir, "voice.wav")

	yamlContent := fmt.Sprintf(`
project: demo
output:
  mode: video
  file: out/demo.mp4
defaults:
  model: %s
  voice: %s
  resolution: 1280x720
  sampler: euler
  steps: 30
  fps: 30
  subject_prompt: "a city"
  negative_prompt: "text"
scenes:
  - id: intro
    prompt: a wide shot
    negative_prompt: blurry
    seed: 42
    tts_text: Welcome
    speed: 1.0
    motion:
      type: zoom_in
      intensity: 0.5
    look:
      grain: 0.3
      white_balance: warm
  - id: breather
    prompt: fog
    silence_seconds: 2.0
`, model, voice)

	path := writeTempYAML(t, yamlContent)
	fromFile, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fromBytes, err := LoadBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if !reflect.DeepEqual(fromFile, fromBytes) {
		t.Errorf("LoadBytes result differs from Load:\nfile:  %+v\nbytes: %+v", fromFile, fromBytes)
	}
	if err := fromBytes.Validate(); err != nil {
		t.Fatalf("Validate on bytes-loaded config: %v", err)
	}

	if _, err := LoadBytes([]byte("scenes: [")); err == nil {
		t.Fatal("expected parse error for malformed YAML, got nil")
	}
}

func TestSeed_ZeroVsAbsent_DistinguishesCorrectly(t *testing.T) {
	yamlZero := `
project: p
scenes:
  - id: a
    prompt: x
    tts_text: y
    seed: 0
`
	yamlAbsent := `
project: p
scenes:
  - id: a
    prompt: x
    tts_text: y
`
	pathZero := writeTempYAML(t, yamlZero)
	pathAbsent := writeTempYAML(t, yamlAbsent)

	cfgZero, err := Load(pathZero)
	if err != nil {
		t.Fatalf("Load zero: %v", err)
	}
	if cfgZero.Scenes[0].Seed == nil {
		t.Error("seed: 0 should produce non-nil *int(0)")
	} else if *cfgZero.Scenes[0].Seed != 0 {
		t.Errorf("seed value = %d, want 0", *cfgZero.Scenes[0].Seed)
	}
	if cfgZero.Scenes[0].ResolvedSeed() != 0 {
		t.Errorf("ResolvedSeed() = %d, want 0", cfgZero.Scenes[0].ResolvedSeed())
	}

	cfgAbsent, err := Load(pathAbsent)
	if err != nil {
		t.Fatalf("Load absent: %v", err)
	}
	if cfgAbsent.Scenes[0].Seed != nil {
		t.Errorf("absent seed should be nil, got %d", *cfgAbsent.Scenes[0].Seed)
	}
	if cfgAbsent.Scenes[0].ResolvedSeed() != -1 {
		t.Errorf("ResolvedSeed() = %d, want -1 (unset sentinel)", cfgAbsent.Scenes[0].ResolvedSeed())
	}
}

func TestValidate_TableDriven_CoversAllInvalidCases(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "model.safetensors")
	voice := touchFile(t, dir, "voice.wav")

	cases := []struct {
		name    string
		cfg     *Config
		wantErr string // substring expected in error; empty means no error
	}{
		{
			name: "valid single scene",
			cfg: &Config{
				Project:    "p",
				OutputFile: "out.mp4",
				Defaults:   Defaults{Model: model, Voice: voice},
				Scenes:     []Scene{{ID: "s1", Prompt: "a", TTSText: "b"}},
			},
			wantErr: "",
		},
		{
			name: "missing output file",
			cfg: &Config{
				Project: "p",
				Scenes:  []Scene{{ID: "s1", Prompt: "a", TTSText: "b"}},
			},
			wantErr: "output_file",
		},
		{
			name: "duplicate id",
			cfg: &Config{
				Project:    "p",
				OutputFile: "out.mp4",
				Scenes: []Scene{
					{ID: "dup", Prompt: "a", TTSText: "b"},
					{ID: "dup", Prompt: "c", TTSText: "d"},
				},
			},
			wantErr: "duplicate scene id",
		},
		{
			name: "empty prompt names scene",
			cfg: &Config{
				Project:    "p",
				OutputFile: "out.mp4",
				Scenes:     []Scene{{ID: "scene42", Prompt: "", TTSText: "b"}},
			},
			wantErr: "scene42",
		},
		{
			name: "empty tts_text names scene",
			cfg: &Config{
				Project:    "p",
				OutputFile: "out.mp4",
				Scenes:     []Scene{{ID: "scene99", Prompt: "a", TTSText: ""}},
			},
			wantErr: "scene99",
		},
		{
			name: "missing model file",
			cfg: &Config{
				Project:    "p",
				OutputFile: "out.mp4",
				Defaults:   Defaults{Model: "/no/such/model.bin"},
				Scenes:     []Scene{{ID: "s1", Prompt: "a", TTSText: "b"}},
			},
			wantErr: "defaults.model",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidate_OnnxVoiceNeedsCompanionConfig(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "m.safetensors")
	voice := touchFile(t, dir, "a.onnx")
	newCfg := func(v string) *Config {
		return &Config{
			Project:    "p",
			OutputFile: "out/p.mp4",
			Defaults:   Defaults{Model: model, Voice: v, Resolution: "512x512", Sampler: "euler", Steps: 4, FPS: 24},
			Scenes:     []Scene{{ID: "s", Prompt: "x", TTSText: "hola", Speed: 1}},
		}
	}

	err := newCfg(voice).Validate()
	if err == nil || !strings.Contains(err.Error(), voice+".json") {
		t.Fatalf("missing companion: error = %v, want one naming %s.json", err, voice)
	}

	touchFile(t, dir, "a.onnx.json")
	if err := newCfg(voice).Validate(); err != nil {
		t.Fatalf("companion present: unexpected error: %v", err)
	}

	// Non-.onnx voices (and images mode, which never narrates) are not subject to the check.
	wav := touchFile(t, dir, "voice.wav")
	if err := newCfg(wav).Validate(); err != nil {
		t.Fatalf("non-.onnx voice: unexpected error: %v", err)
	}
}

func TestValidate_NoVoiceIsNotAValidationError(t *testing.T) {
	// An empty voice is caught at synthesis time with a clear error; Validate
	// must keep accepting it so importer-generated placeholders still validate.
	dir := t.TempDir()
	model := touchFile(t, dir, "m.safetensors")
	cfg := &Config{
		Project:    "p",
		OutputFile: "out/p.mp4",
		Defaults:   Defaults{Model: model, Resolution: "512x512", Sampler: "euler", Steps: 4, FPS: 24},
		Scenes:     []Scene{{ID: "s", Prompt: "x", TTSText: "hola", Speed: 1}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_SceneVoice(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "m.safetensors")
	def := touchFile(t, dir, "d.onnx")
	touchFile(t, dir, "d.onnx.json")
	own := touchFile(t, dir, "o.onnx")
	newCfg := func(v string) *Config {
		return &Config{
			Project:    "p",
			OutputFile: "out/p.mp4",
			Defaults:   Defaults{Model: model, Voice: def, Resolution: "512x512", Sampler: "euler", Steps: 4, FPS: 24},
			Scenes:     []Scene{{ID: "s", Prompt: "x", TTSText: "hola", Speed: 1, Voice: v}},
		}
	}

	if err := newCfg(dir + "/nope.onnx").Validate(); err == nil || !strings.Contains(err.Error(), `scene "s" voice`) {
		t.Fatalf("missing voice: error = %v", err)
	}
	if err := newCfg(own).Validate(); err == nil || !strings.Contains(err.Error(), own+".json") {
		t.Fatalf("missing companion: error = %v", err)
	}
	touchFile(t, dir, "o.onnx.json")
	cfg := newCfg(own)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("own voice present: %v", err)
	}
	if got := cfg.VoiceFor(cfg.Scenes[0]); got != own {
		t.Errorf("VoiceFor = %q, want the scene's %q", got, own)
	}
	if got := newCfg("").VoiceFor(Scene{}); got != def {
		t.Errorf("VoiceFor without own voice = %q, want the default %q", got, def)
	}
}

func TestValidate_SceneSpeaker(t *testing.T) {
	dir := t.TempDir()
	model := touchFile(t, dir, "m.safetensors")
	two := touchFile(t, dir, "two.onnx")
	if err := os.WriteFile(two+".json", []byte(`{"num_speakers": 2, "speaker_id_map": {"M": 0, "F": 1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	one := touchFile(t, dir, "one.onnx")
	touchFile(t, dir, "one.onnx.json")
	n := func(v int) *int { return &v }
	newCfg := func(defSpeaker int, sc Scene) *Config {
		sc.ID, sc.Prompt, sc.TTSText, sc.Speed = "s", "x", "hola", 1
		return &Config{
			Project:    "p",
			OutputFile: "out/p.mp4",
			Defaults:   Defaults{Model: model, Voice: two, VoiceSpeaker: defSpeaker, Resolution: "512x512", Sampler: "euler", Steps: 4, FPS: 24},
			Scenes:     []Scene{sc},
		}
	}

	cfg := newCfg(1, Scene{})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("speaker 1 of a two-speaker voice: %v", err)
	}
	if cfg.SpeakerFor(cfg.Scenes[0]) != 1 {
		t.Error("a scene without its own speaker or voice uses the project's speaker")
	}
	if got := newCfg(1, Scene{Speaker: n(0)}).SpeakerFor(Scene{Speaker: n(0)}); got != 0 {
		t.Errorf("an explicit speaker 0 wins over the project's, got %d", got)
	}
	if got := newCfg(1, Scene{Voice: one}).SpeakerFor(Scene{Voice: one}); got != 0 {
		t.Errorf("a scene with its own voice starts at its first speaker, got %d", got)
	}
	for name, c := range map[string]*Config{
		"default out of range": newCfg(2, Scene{}),
		"scene out of range":   newCfg(0, Scene{Speaker: n(5)}),
		"negative":             newCfg(0, Scene{Speaker: n(-1)}),
		"single-speaker voice": newCfg(0, Scene{Voice: one, Speaker: n(1)}),
	} {
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "speaker") {
			t.Errorf("%s: error = %v, want one about the speaker", name, err)
		}
	}
}
