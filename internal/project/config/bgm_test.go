package config

import (
	"os"
	"path/filepath"
	"testing"
)

// bgmCfg is a valid video project with the given soundtrack and the music
// models installed (so only the block under test can fail).
func bgmCfg(b *BGM) *Config {
	c := musicCfg()
	c.Output = Output{Mode: ModeVideo, File: "out/final.mp4"}
	c.Scenes = []Scene{{ID: "s1", Prompt: "a", TTSText: "b"}}
	c.Tracks = nil
	c.BGM = b
	return c
}

func TestBGM_ValidForms(t *testing.T) {
	dir := taskProject(t) // has sources/base.mp3
	for name, b := range map[string]*BGM{
		"prompt":  {Prompt: "calm piano"},
		"file":    {File: "base.mp3"},
		"all set": {Prompt: "p", Volume: f(0.4), FadeSeconds: f(0), Ducking: new(bool), Seed: new(int)},
	} {
		c := bgmCfg(b)
		c.Defaults.Music.ModelsDir = bgmModels(t)
		if err := c.ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// bgmModels installs the stub files a generated soundtrack needs, apart from
// the language model, which it never uses.
func bgmModels(t *testing.T) string {
	t.Helper()
	d := relDir(t.TempDir())
	for _, n := range []string{DefaultMusicSynth, "Qwen3-Embedding-0.6B-Q8_0.gguf", "vae-BF16.gguf"} {
		os.WriteFile(filepath.Join(d, n), []byte("x"), 0o644)
	}
	return d
}

func TestBGM_Rejections(t *testing.T) {
	dir := taskProject(t)
	cases := map[string]struct {
		b    *BGM
		want string
	}{
		"both":         {&BGM{Prompt: "p", File: "base.mp3"}, "not both"},
		"neither":      {&BGM{}, "set prompt"},
		"blank":        {&BGM{Prompt: "  "}, "set prompt"},
		"missing":      {&BGM{File: "nope.mp3"}, `file "nope.mp3" not found`},
		"path":         {&BGM{File: "../x.mp3"}, "not a path"},
		"volume":       {&BGM{Prompt: "p", Volume: f(1.5)}, "volume must be between 0 and 1"},
		"fade":         {&BGM{Prompt: "p", FadeSeconds: f(-1)}, "fade_seconds must not be negative"},
		"seed on file": {&BGM{File: "base.mp3", Seed: new(int)}, "seed only applies"},
	}
	for name, tc := range cases {
		c := bgmCfg(tc.b)
		c.Defaults.Music.ModelsDir = bgmModels(t)
		wantErr(t, c.ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
}

func TestBGM_OnlyInVideoMode(t *testing.T) {
	for _, mode := range []OutputMode{ModeImages, ModeMusic} {
		c := bgmCfg(&BGM{Prompt: "p"})
		c.Output.Mode = mode
		wantErr(t, c.Validate(), "only works in video mode")
	}
}

func TestBGM_GeneratedNeedsTheMusicModelsButNotTheLanguageModel(t *testing.T) {
	c := bgmCfg(&BGM{Prompt: "p"})
	c.Defaults.Music.ModelsDir = relDir(t.TempDir()) // nothing installed
	wantErr(t, c.ValidateInDir(linked(taskProject(t))), "synthesis model")
	c.Defaults.Music.ModelsDir = bgmModels(t) // no language model in there
	if err := c.ValidateInDir(linked(taskProject(t))); err != nil {
		t.Fatalf("the soundtrack must not need ace-lm's model: %v", err)
	}
	// a file needs no models at all
	c = bgmCfg(&BGM{File: "base.mp3"})
	c.Defaults.Music.ModelsDir = relDir(t.TempDir())
	if err := c.ValidateInDir(linked(taskProject(t))); err != nil {
		t.Fatal(err)
	}
}

func TestBGM_DefaultsAndStableSeed(t *testing.T) {
	b := &BGM{Prompt: "calm piano"}
	if b.ResolvedVolume() != 0.25 || b.ResolvedFade() != 2 || !b.ResolvedDucking() {
		t.Fatalf("%v %v %v", b.ResolvedVolume(), b.ResolvedFade(), b.ResolvedDucking())
	}
	if b.ResolvedSeed() != (&BGM{Prompt: "calm piano"}).ResolvedSeed() || b.ResolvedSeed() == (&BGM{Prompt: "other"}).ResolvedSeed() {
		t.Fatal("the derived seed must follow the prompt")
	}
	s := 5
	if (&BGM{Prompt: "p", Seed: &s}).ResolvedSeed() != 5 {
		t.Fatal("explicit seed wins")
	}
}

func TestBGM_YAML(t *testing.T) {
	c, err := LoadBytes([]byte("project: p\noutput: {file: o.mp4}\nbgm: {prompt: calm, volume: 0.3, ducking: false}\nscenes: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.BGM == nil || c.BGM.Prompt != "calm" || *c.BGM.Volume != 0.3 || c.BGM.ResolvedDucking() {
		t.Fatalf("%+v", c.BGM)
	}
}
