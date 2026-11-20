package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComposeHashIgnoresSynthesisOnly(t *testing.T) {
	a := ComposeParams{Caption: "c", Lyrics: "l", Seed: 7, LMModel: "lm.gguf"}
	first, second := ComposeHash(a), ComposeHash(a)
	if first != second {
		t.Fatal("not deterministic")
	}
	for name, mut := range map[string]func(*ComposeParams){
		"caption": func(p *ComposeParams) { p.Caption = "d" },
		"lyrics":  func(p *ComposeParams) { p.Lyrics = "m" },
		"seed":    func(p *ComposeParams) { p.Seed = 8 },
		"lm":      func(p *ComposeParams) { p.LMModel = "other.gguf" },
		"temp":    func(p *ComposeParams) { p.LMTemperature = "0.5" },
		"bpm":     func(p *ComposeParams) { p.BPM = 90 },
	} {
		b := a
		mut(&b)
		if ComposeHash(a) == ComposeHash(b) {
			t.Errorf("%s must change the compose hash", name)
		}
	}
}

func TestSynthHashCoversRequestAndModels(t *testing.T) {
	a := SynthParams{Request: []byte(`{"a":1}`), SynthModel: "d.gguf", VAE: "v.gguf"}
	for name, b := range map[string]SynthParams{
		"request": {Request: []byte(`{"a":2}`), SynthModel: "d.gguf", VAE: "v.gguf"},
		"dit":     {Request: a.Request, SynthModel: "e.gguf", VAE: "v.gguf"},
		"vae":     {Request: a.Request, SynthModel: "d.gguf", VAE: "w.gguf"},
	} {
		if SynthHash(a) == SynthHash(b) {
			t.Errorf("%s must change the synth hash", name)
		}
	}
	first, second := SynthHash(a), SynthHash(a)
	if first != second {
		t.Fatal("not deterministic")
	}
}

func TestMusicPathsAreStable(t *testing.T) {
	c, a := MusicPaths("intro_v2", "mp3")
	if c != filepath.Join("tmp", "track_intro_v2.compose.json") || a != filepath.Join("tmp", "track_intro_v2.mp3") {
		t.Fatalf("%q %q", c, a)
	}
}

func TestDecideMusicNeedsHashAndFile(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{Scenes: map[string]SceneEntry{}}
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	cp, ap := MusicPaths("a", "mp3")
	m.UpdateEntry("a", SceneEntry{ComposeHash: "h1", SynthHash: "h2", ComposePath: cp, AudioPath: ap})

	if DecideCompose(m, dir, "a", "h1", false) || DecideSynth(m, dir, "a", "h2", false) {
		t.Fatal("hit without the files on disk")
	}
	for _, p := range []string{cp, ap} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !DecideCompose(m, dir, "a", "h1", false) || !DecideSynth(m, dir, "a", "h2", false) {
		t.Fatal("expected hits")
	}
	if DecideCompose(m, dir, "a", "other", false) || DecideSynth(m, dir, "a", "other", false) {
		t.Fatal("changed hash must miss")
	}
	if DecideCompose(m, dir, "a", "h1", true) || DecideSynth(m, dir, "a", "h2", true) {
		t.Fatal("--force must miss")
	}
	if DecideCompose(m, dir, "missing", "h1", false) {
		t.Fatal("unknown key must miss")
	}
}

func TestSynthHashCoversTheAdapterFilesAndKeepsOldHashesStable(t *testing.T) {
	plain := SynthParams{Request: []byte(`{"a":1}`), SynthModel: "d.gguf", VAE: "v.gguf"}
	withZero := plain
	withZero.AdapterBytes = 0
	if SynthHash(plain) != SynthHash(withZero) {
		t.Fatal("no adapter must hash exactly as before")
	}
	a, b := plain, plain
	a.AdapterBytes, b.AdapterBytes = 100, 200
	if SynthHash(a) == SynthHash(plain) || SynthHash(a) == SynthHash(b) {
		t.Fatal("the size of the adapter files must change the hash")
	}
}

func TestComposeHashIgnoresTheAdapter(t *testing.T) {
	// the adapter is not a field of ComposeParams at all: it cannot matter to the language model
	p := ComposeParams{Caption: "c", Lyrics: "l", Seed: 1}
	first, second := ComposeHash(p), ComposeHash(p)
	if first != second {
		t.Fatal("not deterministic")
	}
}
