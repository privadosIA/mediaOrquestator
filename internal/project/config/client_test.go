package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestClientIsKeptAndRoundTrips(t *testing.T) {
	cfg, err := LoadBytes([]byte("client: video-studio\n" + adultBase))
	if err != nil || cfg.Client != "video-studio" {
		t.Fatalf("read %q, %v", cfg.Client, err)
	}
	out, err := yaml.Marshal(cfg)
	if err != nil || !strings.Contains(string(out), "client: video-studio") {
		t.Fatalf("written %q, %v", out, err)
	}
	if none, err := LoadBytes([]byte(adultBase)); err != nil || none.Client != "" {
		t.Fatalf("no client: %q, %v", none.Client, err)
	}
	if out, _ := yaml.Marshal(&Config{Project: "p"}); strings.Contains(string(out), "client:") {
		t.Errorf("an absent client is written: %q", out)
	}
}

func TestClientRefusesWhatIsNotAClientName(t *testing.T) {
	for _, v := range []string{`"Video Studio"`, "VIDEO", `""`, "a_b", strings.Repeat("a", 33), "[a]"} {
		_, err := LoadBytes([]byte("client: " + v + "\n" + adultBase))
		if err == nil || !strings.Contains(err.Error(), "client") {
			t.Errorf("%s: want an error naming the field, got %v", v, err)
		}
	}
	for _, v := range []string{"a", "video-studio", strings.Repeat("a", 32), "0-9"} {
		if _, err := LoadBytes([]byte("client: " + v + "\n" + adultBase)); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
}
