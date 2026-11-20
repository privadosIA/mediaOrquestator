package config

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTags_NormalisedFromATextOrAList(t *testing.T) {
	for name, tc := range map[string]struct{ yaml, want string }{
		"text":    {"tags: \"Piano, Happy\"", "piano,happy"},
		"list":    {"tags: [Piano, Happy Vocals]", "piano,happy vocals"},
		"repeats": {"tags: \"piano,,Piano , happy\"", "piano,happy"},
		"nothing": {"tags: \"\"", ""},
		"spaces":  {"tags: \"  soft   vocals ,piano\"", "soft vocals,piano"},
	} {
		var tr Track
		if err := yaml.Unmarshal([]byte(tc.yaml), &tr); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(tr.Tags) != tc.want {
			t.Errorf("%s: tags = %q, want %q", name, tr.Tags, tc.want)
		}
	}
	var tr Track
	if err := yaml.Unmarshal([]byte("tags: {a: 1}"), &tr); err == nil {
		t.Error("a mapping is not a list of words")
	}
}

func TestTags_RoundTripThroughJSONAndYAML(t *testing.T) {
	var tr Track
	if err := json.Unmarshal([]byte(`{"id":"a","prompt":"p","tags":["Piano","Happy"]}`), &tr); err != nil || tr.Tags != "piano,happy" {
		t.Fatalf("json list: %v %q", err, tr.Tags)
	}
	if err := json.Unmarshal([]byte(`{"id":"a","prompt":"p","tags":"Piano, Happy"}`), &tr); err != nil || tr.Tags != "piano,happy" {
		t.Fatalf("json text: %v %q", err, tr.Tags)
	}
	out, _ := json.Marshal(tr)
	if !strings.Contains(string(out), `"tags":"piano,happy"`) {
		t.Errorf("json out = %s", out)
	}
	y, _ := yaml.Marshal(tr)
	if !strings.Contains(string(y), "tags: piano,happy") {
		t.Errorf("yaml out = %s", y)
	}
	empty, _ := json.Marshal(Track{ID: "a"})
	if strings.Contains(string(empty), "tags") {
		t.Errorf("no tags, no field: %s", empty)
	}
}

func heartMuLaConfig(t *testing.T, tracks ...Track) *Config {
	t.Helper()
	c := &Config{Project: "p", Output: Output{Mode: ModeMusic}, Tracks: tracks}
	c.Defaults.Music = &Music{Generator: GeneratorHeartMuLa}
	return c
}

func TestGenerator_ValidAndInvalid(t *testing.T) {
	c := heartMuLaConfig(t, Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nhola"})
	if err := c.Validate(); err != nil {
		t.Fatalf("heartmula with lyrics: %v", err)
	}
	if !c.Defaults.Music.UsesHeartMuLa() {
		t.Error("UsesHeartMuLa")
	}
	c.Defaults.Music.Generator = "other"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "defaults.music.generator") {
		t.Errorf("an unknown generator must be named: %v", err)
	}
	var none *Music
	if none.ResolvedGenerator() != GeneratorACEStep || none.UsesHeartMuLa() {
		t.Error("no music block: ace-step")
	}
}

func TestGenerator_WhatHeartMuLaCannotDo(t *testing.T) {
	bpm := 90
	c := heartMuLaConfig(t,
		Track{ID: "tarea", Prompt: "p", Task: "cover", Source: "a.mp3"},
		Track{ID: "ref", Prompt: "p", Lyrics: "x", Reference: "r.mp3"},
		Track{ID: "inst", Prompt: "p", Instrumental: true},
		Track{ID: "vacia", Prompt: "p"},
	)
	err := c.Validate()
	if err == nil {
		t.Fatal("tasks, references and tracks without lyrics must fail")
	}
	for _, want := range []string{`track "tarea"`, `track "ref"`, `track "inst"`, `track "vacia"`, "not supported by the heartmula generator", "needs lyrics"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	w := heartMuLaConfig(t, Track{ID: "a", Prompt: "p", Lyrics: "x", BPM: &bpm, KeyScale: "C major", TimeSignature: "4"})
	if err := w.Validate(); err != nil {
		t.Fatalf("ignored fields are only warnings: %v", err)
	}
	if len(w.Warnings) == 0 || !strings.Contains(strings.Join(w.Warnings, "\n"), "bpm, keyscale, timesignature are ignored by the heartmula generator") {
		t.Errorf("warnings = %v", w.Warnings)
	}
}

func TestGenerator_ACEStepProjectsAreUnchanged(t *testing.T) {
	bpm := 90
	c := &Config{Project: "p", Output: Output{Mode: ModeMusic}, Tracks: []Track{{ID: "a", Prompt: "p", BPM: &bpm, Task: "cover", Source: "x.mp3"}}}
	c.Defaults.Music = &Music{}
	// Whatever else is wrong with it (the source is not there), nothing about heartmula is said.
	if err := c.Validate(); err != nil && strings.Contains(err.Error(), "heartmula") {
		t.Errorf("an ACE-Step project must not hear of heartmula: %v", err)
	}
	for _, w := range c.Warnings {
		if strings.Contains(w, "heartmula") {
			t.Errorf("warning: %s", w)
		}
	}
}
