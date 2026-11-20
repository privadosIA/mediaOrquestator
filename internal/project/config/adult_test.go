package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const adultBase = "project: p\noutput:\n  mode: video\n  file: out.mp4\nscenes:\n  - id: a\n    prompt: x\n    tts_text: t\n"

func TestAdultLabelHasThreeStates(t *testing.T) {
	for name, tc := range map[string]struct {
		extra string
		want  string // "", "true" or "false"
	}{
		"absent": {"", ""},
		"true":   {"adult: true\n", "true"},
		"false":  {"adult: false\n", "false"},
	} {
		cfg, err := LoadBytes([]byte(tc.extra + adultBase))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := ""
		if cfg.Adult != nil {
			got = map[bool]string{true: "true", false: "false"}[*cfg.Adult]
		}
		if got != tc.want {
			t.Errorf("%s: read %q, want %q", name, got, tc.want)
		}
		// saving keeps the state: nothing adds, removes or flips it
		out, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if has := strings.Contains(string(out), "adult:"); has != (tc.want != "") {
			t.Errorf("%s: written %q", name, out)
		}
		back, err := LoadBytes(out)
		if err != nil || (back.Adult == nil) != (cfg.Adult == nil) || (cfg.Adult != nil && *back.Adult != *cfg.Adult) {
			t.Errorf("%s: round trip lost the state: %v %v", name, back, err)
		}
	}
}

func TestAdultLabelRefusesOtherValues(t *testing.T) {
	_, err := LoadBytes([]byte("adult: maybe\n" + adultBase))
	if err == nil || !strings.Contains(err.Error(), "adult") {
		t.Fatalf("want an error naming the field, got %v", err)
	}
}
