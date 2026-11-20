package config

import (
	"strings"
	"testing"
)

func dr2Config(tracks ...Track) *Config {
	c := &Config{Project: "p", Output: Output{Mode: ModeMusic}, Tracks: tracks}
	c.Defaults.Music = &Music{Generator: GeneratorDiffRhythm2}
	return c
}

var dr2Lyrics = Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nhello there", Tags: "pop,piano"}

func TestDiffRhythm2Generator_ValidAndUsesService(t *testing.T) {
	c := dr2Config(dr2Lyrics)
	if err := c.Validate(); err != nil {
		t.Fatalf("diffrhythm2 with English lyrics: %v", err)
	}
	m := c.Defaults.Music
	if !m.UsesDiffRhythm2() || m.UsesHeartMuLa() || !m.UsesService() {
		t.Error("the generator flags")
	}
	if c.UsesLM() {
		t.Error("diffrhythm2 writes the song itself: no ACE-Step language model")
	}
	var none *Music
	if none.UsesDiffRhythm2() || none.UsesService() {
		t.Error("no music block: ace-step")
	}
}

func TestDiffRhythm2Generator_WhatItCannotDoIsAnError(t *testing.T) {
	for name, tc := range map[string]struct {
		track Track
		want  string
	}{
		"spanish":      {Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nhola", VocalLanguage: "es"}, "English lyrics only"},
		"instrumental": {Track{ID: "a", Prompt: "p", Instrumental: true}, "instrumentals"},
		"no lyrics":    {Track{ID: "a", Prompt: "p"}, "needs lyrics"},
		"remix":        {Track{ID: "a", Prompt: "p", Task: TaskRepaint, Source: "s.mp3"}, "tasks"},
		"reference":    {Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nhi", Reference: "r.mp3"}, "reference"},
	} {
		err := dr2Config(tc.track).Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `"a"`) {
			t.Errorf("%s: want an error naming the track and %q, got %v", name, tc.want, err)
		}
	}
}

func TestDiffRhythm2Generator_EnglishIsAcceptedAndTheDefaultVoiceLanguageCounts(t *testing.T) {
	tr := dr2Lyrics
	tr.VocalLanguage = "en"
	if err := dr2Config(tr).Validate(); err != nil {
		t.Errorf("en: %v", err)
	}
	c := dr2Config(dr2Lyrics)
	c.Defaults.Music.VocalLanguage = "es"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "English lyrics only") {
		t.Errorf("the project's language is Spanish: %v", err)
	}
}

func TestDiffRhythm2Generator_TheFieldsItIgnoresAreWarnedAbout(t *testing.T) {
	tr := dr2Lyrics
	bpm := 90
	tr.BPM = &bpm
	tr.KeyScale = "C major"
	tr.Vocals = "female"
	c := dr2Config(tr)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(c.Warnings, "\n")
	for _, f := range []string{"bpm", "keyscale", "vocals"} {
		if !strings.Contains(joined, f) || !strings.Contains(joined, "diffrhythm2") {
			t.Errorf("a warning names %s: %q", f, joined)
		}
	}
}

func TestDiffRhythm2Generator_TheSamplingFieldsAreChecked(t *testing.T) {
	for name, set := range map[string]func(m *Music){
		"low guidance":  func(m *Music) { v := 0.1; m.DR2CfgStrength = &v },
		"high steps":    func(m *Music) { v := 200; m.DR2Steps = &v },
		"too few steps": func(m *Music) { v := 1; m.DR2Steps = &v },
	} {
		c := dr2Config(dr2Lyrics)
		set(c.Defaults.Music)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "dr2_") {
			t.Errorf("%s: %v", name, err)
		}
	}
	c := dr2Config(dr2Lyrics)
	g, n := 2.5, 24
	c.Defaults.Music.DR2CfgStrength, c.Defaults.Music.DR2Steps = &g, &n
	if err := c.Validate(); err != nil {
		t.Errorf("good values: %v", err)
	}
	// another generator warns instead of failing
	h := heartMuLaConfig(t, Track{ID: "a", Prompt: "p", Lyrics: "[Verse]\nhi", Tags: "pop"})
	h.Defaults.Music.DR2Steps = &n
	if err := h.Validate(); err != nil || !strings.Contains(strings.Join(h.Warnings, "\n"), "dr2_") {
		t.Errorf("heartmula with dr2_steps: %v %v", err, h.Warnings)
	}
}

func TestGenerator_AnUnknownOneNamesTheThree(t *testing.T) {
	c := dr2Config(dr2Lyrics)
	c.Defaults.Music.Generator = "other"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), GeneratorDiffRhythm2) {
		t.Errorf("the message lists the generators: %v", err)
	}
}
