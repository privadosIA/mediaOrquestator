package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// musicCfg builds a music config whose models directory holds every model
// file, so tests of other rules are not tripped by the model check.
func musicCfg(tracks ...Track) *Config {
	dir, err := os.MkdirTemp(".", "music-models")
	if err != nil {
		panic(err)
	}
	for _, f := range []string{DefaultMusicLM, DefaultMusicSynth, "Qwen3-Embedding-0.6B-Q8_0.gguf", "vae-BF16.gguf"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			panic(err)
		}
	}
	return &Config{
		Project:  "songs",
		Output:   Output{Mode: ModeMusic},
		Defaults: Defaults{Music: &Music{ModelsDir: dir}},
		Tracks:   tracks,
	}
}

func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("error = %v, want containing %q", err, sub)
	}
}

func TestMusicMinimalTrackIsValid(t *testing.T) {
	c := musicCfg(Track{ID: "intro", Prompt: "calm piano"})
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestMusicModeNeedsNoOutputFile(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	if err := c.Validate(); err != nil {
		t.Fatalf("music without output.file: %v", err)
	}
}

func TestMusicTrackWithoutPrompt(t *testing.T) {
	wantErr(t, musicCfg(Track{ID: "a"}).Validate(), `track "a" has an empty prompt`)
}

func TestMusicScenesRejected(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Scenes = []Scene{{ID: "s", Prompt: "p"}}
	wantErr(t, c.Validate(), "uses tracks, not scenes")
}

func TestMusicNeedsTracks(t *testing.T) {
	wantErr(t, musicCfg().Validate(), "at least one track")
}

func TestTracksRejectedOutsideMusic(t *testing.T) {
	for _, mode := range []OutputMode{ModeVideo, ModeImages} {
		c := &Config{Project: "p", Output: Output{Mode: mode, File: "out/x.mp4"}, Tracks: []Track{{ID: "a", Prompt: "p"}}}
		wantErr(t, c.Validate(), "tracks is only valid")
	}
}

func TestMusicDuplicateTrackID(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"}, Track{ID: "a", Prompt: "q"})
	wantErr(t, c.Validate(), `duplicate track id "a"`)
}

func TestMusicInvalidTrackID(t *testing.T) {
	wantErr(t, musicCfg(Track{ID: "../x", Prompt: "p"}).Validate(), "invalid id")
}

func TestMusicInvalidOutputFormat(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.Music.OutputFormat = "ogg"
	err := c.Validate()
	wantErr(t, err, `"mp3" or "wav"`)
}

func TestMusicIgnoredImageDefaultsWarn(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.SubjectPrompt = "x"
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "subject_prompt") {
		t.Fatalf("warnings = %v", c.Warnings)
	}
}

func TestMusicVariantsAndPathCollision(t *testing.T) {
	one := 1
	two := 2
	c := musicCfg(Track{ID: "a_v1", Prompt: "p", Variants: &one}, Track{ID: "a", Prompt: "p", Variants: &two})
	wantErr(t, c.Validate(), "both resolve to output path")
}

func TestMusicBadTrackNumbers(t *testing.T) {
	zero, neg := 0, -3.0
	c := musicCfg(Track{ID: "a", Prompt: "p", Variants: &zero, DurationSeconds: &neg, BPM: &zero})
	err := c.Validate()
	wantErr(t, err, "variants must be")
	wantErr(t, err, "duration_seconds must be > 0")
	wantErr(t, err, "bpm must be > 0")
}

func TestMusicInstrumentalWithLyricsWarns(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Lyrics: "la la", Instrumental: true})
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "instrumental") {
		t.Fatalf("warnings = %v", c.Warnings)
	}
}

func TestEffectiveUseLM(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		tr   Track
		want bool
	}{
		{"caption only", Track{Prompt: "p"}, true},
		{"own lyrics", Track{Prompt: "p", Lyrics: "[Verse]\nhola"}, false},
		{"blank lyrics", Track{Prompt: "p", Lyrics: "  \n"}, true},
		{"lyrics forced on", Track{Prompt: "p", Lyrics: "x", UseLM: &yes}, true},
		{"caption forced off", Track{Prompt: "p", UseLM: &no}, false},
		{"instrumental", Track{Prompt: "p", Instrumental: true, Lyrics: "x"}, true},
	}
	for _, c := range cases {
		if got := c.tr.EffectiveUseLM(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestUsesLM(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Lyrics: "x"})
	if c.UsesLM() {
		t.Fatal("own-lyrics project should not use the LM")
	}
	c.Tracks = append(c.Tracks, Track{ID: "b", Prompt: "p"})
	if !c.UsesLM() {
		t.Fatal("caption-only track should use the LM")
	}
}

func TestVariantAudioName(t *testing.T) {
	if got := VariantAudioName("a", 1, 0, "mp3"); got != "a.mp3" {
		t.Fatal(got)
	}
	if got := VariantAudioName("a", 3, 1, "wav"); got != "a_v2.wav" {
		t.Fatal(got)
	}
}

func TestMusicYAMLRoundTrip(t *testing.T) {
	cfg, err := LoadBytes([]byte(`project: songs
output: {mode: music}
defaults:
  music: {output_format: wav, lm_top_k: 0, shift: 3}
tracks:
  - id: intro
    prompt: calm piano
    lyrics: |
      [Verse]
      hola
    seed: 0
    use_lm: false
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Defaults.Music.ModelsDir = musicCfg().Defaults.Music.ModelsDir
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	m := cfg.Defaults.Music
	if m.LMTopK == nil || *m.LMTopK != 0 || m.Shift == nil || *m.Shift != 3 {
		t.Fatalf("explicit zero lost: %+v", m)
	}
	tr := cfg.Tracks[0]
	if tr.Seed == nil || tr.ResolvedSeed() != 0 || tr.EffectiveUseLM() {
		t.Fatalf("track = %+v", tr)
	}
}

func TestMusicMissingModelsPointToTheCatalog(t *testing.T) {
	c := &Config{Project: "songs", Output: Output{Mode: ModeMusic},
		Defaults: Defaults{Music: &Music{ModelsDir: relDir(t.TempDir())}},
		Tracks:   []Track{{ID: "a", Prompt: "p"}}}
	err := c.Validate()
	wantErr(t, err, DefaultMusicSynth)
	wantErr(t, err, DefaultMusicLM)
	wantErr(t, err, "text encoder")
	wantErr(t, err, "VAE")
	wantErr(t, err, "Music tab")
}

func TestMusicLanguageModelOnlyNeededWhenUsed(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nla"})
	if err := os.Remove(filepath.Join(c.Defaults.Music.ModelsDir, DefaultMusicLM)); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("own-lyrics project must not need the LM: %v", err)
	}
	c.Tracks = append(c.Tracks, Track{ID: "b", Prompt: "p"})
	wantErr(t, c.Validate(), DefaultMusicLM)
}

func TestMusicModelNamesGetTheGGUFSuffix(t *testing.T) {
	m := &Music{SynthModel: "acestep-v15-sft-Q8_0", LMModel: "x.gguf"}
	if m.ResolvedSynth() != "acestep-v15-sft-Q8_0.gguf" || m.ResolvedLM() != "x.gguf" {
		t.Fatalf("%q %q", m.ResolvedSynth(), m.ResolvedLM())
	}
	var nilMusic *Music
	if nilMusic.ResolvedLM() != DefaultMusicLM || nilMusic.ResolvedSynth() != DefaultMusicSynth {
		t.Fatal("defaults")
	}
}

func TestMusicCaption(t *testing.T) {
	cases := []struct {
		name   string
		m      *Music
		prompt string
		want   string
	}{
		{"no music block", nil, "piano", "piano"},
		{"no prefix", &Music{}, "piano", "piano"},
		{"blank prefix", &Music{StylePrefix: "  \n "}, "piano", "piano"},
		{"prefix", &Music{StylePrefix: "warm analog"}, "piano", "warm analog, piano"},
		{"prefix is trimmed", &Music{StylePrefix: "  warm analog  "}, "piano", "warm analog, piano"},
	}
	for _, c := range cases {
		if got := c.m.Caption(c.prompt); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMusicStylePrefixIsAccepted(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.Music.StylePrefix = "warm analog production"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ndefaults:\n  music: {style_prefix: warm}\ntracks:\n  - {id: a, prompt: x}\n"))
	if err != nil || cfg.Defaults.Music.StylePrefix != "warm" {
		t.Fatalf("%+v %v", cfg, err)
	}
}

func withAdapter(t *testing.T, c *Config, names ...string) {
	t.Helper()
	dir := filepath.Join(c.Defaults.Music.ModelsDir, AdaptersSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveAdapter(t *testing.T) {
	half, one := 0.5, 1.0
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	if n, s := c.ResolveAdapter(c.Tracks[0]); n != "" || s != 1 {
		t.Fatalf("none: %q %v", n, s)
	}
	c.Defaults.Music.Adapter, c.Defaults.Music.AdapterScale = "style", &half
	if n, s := c.ResolveAdapter(Track{}); n != "style" || s != 0.5 {
		t.Fatalf("default: %q %v", n, s)
	}
	if n, s := c.ResolveAdapter(Track{Adapter: "other"}); n != "other" || s != 0.5 {
		t.Fatalf("own adapter inherits the default scale: %q %v", n, s)
	}
	if n, s := c.ResolveAdapter(Track{AdapterScale: &one}); n != "style" || s != 1 {
		t.Fatalf("own scale: %q %v", n, s)
	}
	zero := 0.0
	if _, s := c.ResolveAdapter(Track{AdapterScale: &zero}); s != 0 {
		t.Fatalf("an explicit zero scale must survive, got %v", s)
	}
	var nilCfg = &Config{}
	if n, s := nilCfg.ResolveAdapter(Track{}); n != "" || s != 1 {
		t.Fatalf("no music block: %q %v", n, s)
	}
}

func TestMusicAdapterMustBeInstalled(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Adapter: "chinese-new-year"})
	err := c.Validate()
	wantErr(t, err, `adapter "chinese-new-year" not found`)
	wantErr(t, err, "Music tab")
	withAdapter(t, c, "chinese-new-year") // a folder or a file: the name is what matters
	if err := c.Validate(); err != nil {
		t.Fatalf("installed adapter: %v", err)
	}
}

func TestMusicDefaultAdapterAppliesToTracksThatSetNone(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"}, Track{ID: "b", Prompt: "q"})
	c.Defaults.Music.Adapter = "style.safetensors"
	wantErr(t, c.Validate(), `adapter "style.safetensors" not found`)
	withAdapter(t, c, "style.safetensors")
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMusicAdapterIsReportedOncePerName(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"}, Track{ID: "b", Prompt: "q"})
	c.Defaults.Music.Adapter = "x"
	if n := strings.Count(c.Validate().Error(), `adapter "x" not found`); n != 1 {
		t.Fatalf("reported %d times", n)
	}
}

func TestMusicAdapterScaleMustNotBeNegative(t *testing.T) {
	neg := -1.0
	c := musicCfg(Track{ID: "a", Prompt: "p", AdapterScale: &neg})
	wantErr(t, c.Validate(), `track "a": adapter_scale must not be negative`)
	c = musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.Music.AdapterScale = &neg
	wantErr(t, c.Validate(), "defaults.music.adapter_scale must not be negative")
}

func TestMusicAdapterNameIsNotAPath(t *testing.T) {
	for _, name := range []string{"../x", "a/b", ".."} {
		c := musicCfg(Track{ID: "a", Prompt: "p", Adapter: name})
		wantErr(t, c.Validate(), "must be a file or folder name")
	}
}

func TestMusicAdapterYAMLRoundTrip(t *testing.T) {
	cfg, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ndefaults:\n  music: {adapter: style, adapter_scale: 0.7}\ntracks:\n  - {id: a, prompt: x, adapter_scale: 0}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n, s := cfg.ResolveAdapter(cfg.Tracks[0]); n != "style" || s != 0 {
		t.Fatalf("%q %v", n, s)
	}
}

func TestMusicEngine(t *testing.T) {
	no := false
	for name, tc := range map[string]struct {
		m    Music
		want string
	}{
		"default":       {Music{}, ""},
		"cli":           {Music{Engine: "cli"}, ""},
		"server":        {Music{Engine: "server"}, ""},
		"server unload": {Music{Engine: "server", KeepLoaded: &no}, ""},
		"unknown":       {Music{Engine: "remote"}, `engine "remote" is invalid`},
		"keep w/o srv":  {Music{KeepLoaded: &no}, "keep_loaded only applies"},
	} {
		c := musicCfg(Track{ID: "a", Prompt: "p"})
		tc.m.ModelsDir = c.Defaults.Music.ModelsDir
		c.Defaults.Music = &tc.m
		err := c.Validate()
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		wantErr(t, err, tc.want)
	}
	var m *Music
	if m.ResolvedEngine() != EngineCLI || !m.ResolvedKeepLoaded() {
		t.Fatal("nil defaults")
	}
	no = false
	if (&Music{Engine: "server", KeepLoaded: &no}).ResolvedKeepLoaded() {
		t.Fatal("explicit false")
	}
}

func TestMusicVocalsAreValidated(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Vocals: "female"})
	c.Defaults.Music.Vocals = "choir"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c = musicCfg(Track{ID: "a", Prompt: "p", Vocals: "yoda"})
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), `vocals "yoda" is invalid`) {
		t.Fatalf("track: %v", err)
	}
	c = musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.Music.Vocals = "yoda"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "defaults.music.vocals") {
		t.Fatalf("default: %v", err)
	}
	cfg, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ntracks:\n  - {id: a, prompt: x, vocals: child}\n"))
	if err != nil || cfg.Tracks[0].Vocals != "child" {
		t.Fatalf("%+v %v", cfg, err)
	}
}

func TestMixMasterGainIsValidated(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Mixes = []Mix{{ID: "m", Clips: []Clip{{Track: "a"}}, Gain: fp(0.7), Normalize: true}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, g := range []float64{-0.1, 2.5} {
		c.Mixes[0].Gain = fp(g)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), `mix "m": gain must be between 0 and 2`) {
			t.Fatalf("gain %v: %v", g, err)
		}
	}
	cfg, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ntracks:\n  - {id: a, prompt: x}\nmixes:\n  - {id: m, gain: 0.5, normalize: true, clips: [{track: a}]}\n"))
	if err != nil || cfg.Mixes[0].Gain == nil || *cfg.Mixes[0].Gain != 0.5 || !cfg.Mixes[0].Normalize {
		t.Fatalf("%+v %v", cfg, err)
	}
}

func TestFavoritesAreValidated(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Favorites = []Favorite{{Track: "a", Seed: 7}, {Track: "a", Seed: 7}, {Track: "gone", Seed: 1}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(c.Warnings, "|")
	if !strings.Contains(joined, "listed more than once") || !strings.Contains(joined, `"gone"`) {
		t.Fatalf("warnings = %v", c.Warnings)
	}
	c.Favorites = []Favorite{{Seed: 3}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "has no track") {
		t.Fatalf("empty track: %v", err)
	}
	cfg, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ntracks:\n  - {id: a, prompt: x}\nfavorites:\n  - {track: a, seed: 99}\n"))
	if err != nil || len(cfg.Favorites) != 1 || cfg.Favorites[0] != (Favorite{Track: "a", Seed: 99}) {
		t.Fatalf("%+v %v", cfg, err)
	}
}
