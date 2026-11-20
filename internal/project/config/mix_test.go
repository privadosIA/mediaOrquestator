package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func mixCfg(mixes ...Mix) *Config {
	c := musicCfg(Track{ID: "base", Prompt: "p"}, Track{ID: "voz", Prompt: "p", Variants: ip(2)})
	c.Mixes = mixes
	return c
}

func TestMixLayeringIsValid(t *testing.T) {
	c := mixCfg(Mix{ID: "final", Clips: []Clip{
		{Track: "base"},
		{Track: "voz", Variant: ip(2), Start: fp(10), Gain: fp(0.8), FadeIn: fp(1), TrimStart: fp(1.7), TrimEnd: fp(24)},
		{File: "efecto.mp3", Start: fp(5), Mute: true},
	}})
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("warnings: %v", c.Warnings)
	}
}

func TestMixDefaultsAndResolvedFiles(t *testing.T) {
	cl := Clip{Track: "base"}
	if cl.ResolvedStart() != 0 || cl.ResolvedGain() != 1 || cl.ResolvedFadeIn() != 0 || cl.ResolvedFadeOut() != 0 || cl.ResolvedVariant() != 1 {
		t.Fatalf("defaults: %+v", cl)
	}
	c := mixCfg(Mix{ID: "final", Clips: []Clip{cl}})
	if got := c.MixFile("final"); got != "out/songs/final.mp3" {
		t.Fatalf("MixFile = %q", got)
	}
	if got, ok := c.TrackFile("base", 1); !ok || got != "out/songs/base.mp3" {
		t.Fatalf("TrackFile base = %q %v", got, ok)
	}
	if got, ok := c.TrackFile("voz", 2); !ok || got != "out/songs/voz_v2.mp3" {
		t.Fatalf("TrackFile voz v2 = %q %v", got, ok)
	}
	if _, ok := c.TrackFile("voz", 3); ok {
		t.Fatal("variant 3 does not exist")
	}
}

func TestMixRules(t *testing.T) {
	cases := []struct {
		name string
		mix  Mix
		want string
	}{
		{"no clips", Mix{ID: "m"}, `mix "m" has no clips`},
		{"bad id", Mix{ID: "a b", Clips: []Clip{{Track: "base"}}}, "invalid id"},
		{"empty id", Mix{Clips: []Clip{{Track: "base"}}}, "invalid id"},
		{"id of a track", Mix{ID: "base", Clips: []Clip{{Track: "base"}}}, "same id as a track"},
		{"both sources", Mix{ID: "m", Clips: []Clip{{Track: "base", File: "a.mp3"}}}, `clip 1: track and file cannot both be set`},
		{"no source", Mix{ID: "m", Clips: []Clip{{Start: fp(1)}}}, `clip 1: needs a track`},
		{"unknown track", Mix{ID: "m", Clips: []Clip{{Track: "nope"}}}, `track "nope" is not a track`},
		{"unknown variant", Mix{ID: "m", Clips: []Clip{{Track: "voz", Variant: ip(3)}}}, `track "voz" has no variant 3`},
		{"variant zero", Mix{ID: "m", Clips: []Clip{{Track: "voz", Variant: ip(0)}}}, "variant"},
		{"variant on a file", Mix{ID: "m", Clips: []Clip{{File: "a.mp3", Variant: ip(1)}}}, "variant only applies to a clip with a track"},
		{"file is a path", Mix{ID: "m", Clips: []Clip{{File: "../a.mp3"}}}, "not a path"},
		{"file wrong type", Mix{ID: "m", Clips: []Clip{{File: "a.ogg"}}}, "not a path"},
		{"negative start", Mix{ID: "m", Clips: []Clip{{Track: "base", Start: fp(-1)}}}, "start must be >= 0"},
		{"gain too high", Mix{ID: "m", Clips: []Clip{{Track: "base", Gain: fp(2.5)}}}, "gain must be between 0 and 2"},
		{"gain negative", Mix{ID: "m", Clips: []Clip{{Track: "base", Gain: fp(-0.1)}}}, "gain must be between 0 and 2"},
		{"negative fade in", Mix{ID: "m", Clips: []Clip{{Track: "base", FadeIn: fp(-1)}}}, "fade_in must be >= 0"},
		{"negative fade out", Mix{ID: "m", Clips: []Clip{{Track: "base", FadeOut: fp(-1)}}}, "fade_out must be >= 0"},
		{"negative trim start", Mix{ID: "m", Clips: []Clip{{Track: "base", TrimStart: fp(-1)}}}, "trim_start must be >= 0"},
		{"trim end before start", Mix{ID: "m", Clips: []Clip{{Track: "base", TrimStart: fp(5), TrimEnd: fp(5)}}}, "trim_end (5) must be greater than trim_start (5)"},
	}
	for _, c := range cases {
		wantErr(t, mixCfg(c.mix).Validate(), c.want)
	}
}

func TestMixDuplicateIDsAndNamesTheClip(t *testing.T) {
	err := mixCfg(Mix{ID: "m", Clips: []Clip{{Track: "base"}}}, Mix{ID: "m", Clips: []Clip{{Track: "base"}}}).Validate()
	wantErr(t, err, `duplicate mix id "m"`)
	err = mixCfg(Mix{ID: "final", Clips: []Clip{{Track: "base"}, {Track: "nope"}}}).Validate()
	wantErr(t, err, `mix "final" clip 2`)
}

func TestMixFadesLongerThanTheClipOnlyWarn(t *testing.T) {
	c := mixCfg(Mix{ID: "m", Clips: []Clip{{Track: "base", TrimStart: fp(0), TrimEnd: fp(4), FadeIn: fp(3), FadeOut: fp(3)}}})
	if err := c.Validate(); err != nil {
		t.Fatalf("must only warn: %v", err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "fades") {
		t.Fatalf("warnings = %v", c.Warnings)
	}
}

func TestMixOnlyInMusicMode(t *testing.T) {
	c := &Config{Project: "v", Output: Output{Mode: ModeVideo, File: "out/v.mp4"}, Scenes: []Scene{{ID: "a", Prompt: "p", TTSText: "hi"}},
		Mixes: []Mix{{ID: "m", Clips: []Clip{{File: "a.mp3"}}}}}
	wantErr(t, c.Validate(), "mixes is only valid with output.mode")
}

func TestMixFileMayNotTakeATrackPath(t *testing.T) {
	// the track "voz" with 2 variants exports voz_v1.mp3 and voz_v2.mp3
	c := mixCfg(Mix{ID: "voz_v1", Clips: []Clip{{Track: "base"}}})
	wantErr(t, c.Validate(), `both resolve to output path "out/songs/voz_v1.mp3"`)
}

func TestMixYAMLRoundTrip(t *testing.T) {
	src := `
project: songs
output: {mode: music}
tracks:
  - {id: base, prompt: p}
mixes:
  - id: final
    clips:
      - track: base
      - file: voz.mp3
        start: 10.5
        gain: 0.8
        mute: true
        fade_in: 1
        fade_out: 2
        trim_start: 1.7
        trim_end: 24.4
`
	var c Config
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	m := c.Mixes[0]
	cl := m.Clips[1]
	if m.ID != "final" || len(m.Clips) != 2 || m.Clips[0].Track != "base" || cl.File != "voz.mp3" ||
		cl.ResolvedStart() != 10.5 || cl.ResolvedGain() != 0.8 || !cl.Mute || cl.ResolvedFadeIn() != 1 || cl.ResolvedFadeOut() != 2 ||
		*cl.TrimStart != 1.7 || *cl.TrimEnd != 24.4 {
		t.Fatalf("parsed = %+v", m)
	}
}
