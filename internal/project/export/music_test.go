package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMusicManifestRoundTripAndAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	m := MusicManifest{
		Project: "songs", Mode: "music", GeneratedAt: "2026-10-10T00:00:00Z",
		Tracks: []MusicEntry{{
			TrackID: "a", Variant: 1, EffectiveSeed: 42, UseLM: true,
			Caption: "final caption", LyricsRequested: "x", LyricsSung: "y",
			SynthModel: "dit.gguf", Params: map[string]any{"inference_steps": 8}, File: "out/songs/a_v2.mp3",
		}},
	}
	if err := WriteMusicManifest(path, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
	var got MusicManifest
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	e := got.Tracks[0]
	if got.Mode != "music" || e.LyricsRequested != "x" || e.LyricsSung != "y" || e.EffectiveSeed != 42 || e.Params["inference_steps"].(float64) != 8 {
		t.Fatalf("round trip: %+v", got)
	}
}

func TestCopyFileCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mp3")
	os.WriteFile(src, []byte("audio"), 0o644)
	dst := filepath.Join(dir, "out", "songs", "a.mp3")
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "audio" {
		t.Fatalf("copy = %q", b)
	}
	if err := CopyFile(filepath.Join(dir, "missing"), dst); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestRejectCollisions(t *testing.T) {
	err := RejectCollisions([]MusicEntry{{TrackID: "a_v1", File: "out/p/a_v1.mp3"}, {TrackID: "a", File: "out/p/a_v1.mp3"}})
	if err == nil || !strings.Contains(err.Error(), "both resolve") {
		t.Fatalf("err = %v", err)
	}
	if err := RejectCollisions([]MusicEntry{{TrackID: "a", File: "x"}, {TrackID: "a", File: "x"}}); err != nil {
		t.Fatal("same track twice is not a collision")
	}
}

func TestReadMusicManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	m := MusicManifest{Project: "p", Mode: "music", Tracks: []MusicEntry{{TrackID: "a", File: "x"}}}
	if err := WriteMusicManifest(path, m); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMusicManifest(path)
	if err != nil || len(got.Tracks) != 1 || got.Tracks[0].TrackID != "a" {
		t.Fatalf("%+v %v", got, err)
	}
	for name, content := range map[string]string{"garbage": "{nope", "images": `{"mode":"images"}`, "empty": ""} {
		os.WriteFile(path, []byte(content), 0o644)
		if _, err := ReadMusicManifest(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := ReadMusicManifest(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing file must be an error")
	}
}
