package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMusicDisplayTitle(t *testing.T) {
	cases := map[string]Track{
		"Night Drive":   {ID: "night_drive", Prompt: "p", Title: "  Night Drive "},
		"night drive":   {ID: "night_drive", Prompt: "p"},
		"intro - 2 min": {ID: "intro-_-2_min", Prompt: "p"},
	}
	for want, tr := range cases {
		if got := tr.DisplayTitle(); got != want && (want != "intro - 2 min" || got != "intro 2 min") {
			t.Errorf("DisplayTitle(%+v) = %q, want %q", tr, got, want)
		}
	}
}

func TestMusicCoverModeOfATrack(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	if got := c.CoverMode(c.Tracks[0]); got != CoverOff {
		t.Errorf("no cover configured = %q", got)
	}
	c.Defaults.Music.Cover = CoverAuto
	if got := c.CoverMode(c.Tracks[0]); got != CoverAuto {
		t.Errorf("project auto = %q", got)
	}
	if got := c.CoverMode(Track{ID: "b", Prompt: "p", Cover: CoverOff}); got != CoverOff {
		t.Errorf("track off wins = %q", got)
	}
	if got := c.CoverMode(Track{ID: "b", Prompt: "p", Cover: "mine.png"}); got != "file" {
		t.Errorf("own file wins = %q", got)
	}
}

func TestMusicCoverAutoNeedsAnImageModel(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	c.Defaults.Music.Cover = CoverAuto
	wantErr(t, c.Validate(), "needs an image model")
	model := relDir(t.TempDir()) + "/m.safetensors"
	if err := os.WriteFile(model, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Defaults.Model = model
	if err := c.Validate(); err != nil {
		t.Fatalf("with a model: %v", err)
	}
	c.Defaults.Music.Cover = "sometimes"
	wantErr(t, c.Validate(), "is invalid")
}

func TestMusicTrackCoverIsOffOrAPictureOfSources(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p", Cover: "off"})
	if err := c.Validate(); err != nil {
		t.Fatalf("off: %v", err)
	}
	c.Tracks[0].Cover = "song.mp3"
	wantErr(t, c.Validate(), "must be")
	c.Tracks[0].Cover = "mine.png"
	wantErr(t, c.Validate(), "not found in sources")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, SourcesSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SourcesSubdir, "mine.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.ValidateInDir(linked(dir)); err != nil {
		t.Fatalf("with the picture: %v", err)
	}
}

func TestMusicLoudnessMustBeInRange(t *testing.T) {
	c := musicCfg(Track{ID: "a", Prompt: "p"})
	for _, ok := range []float64{-14, -30, -8} {
		l := ok
		c.Defaults.Music.LoudnessLUFS = &l
		if err := c.Validate(); err != nil {
			t.Fatalf("%g: %v", ok, err)
		}
	}
	for _, bad := range []float64{-5, -40, 0} {
		l := bad
		c.Defaults.Music.LoudnessLUFS = &l
		wantErr(t, c.Validate(), "loudness_lufs")
	}
}
