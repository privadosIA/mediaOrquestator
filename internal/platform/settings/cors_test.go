package settings

import (
	"os"
	"testing"
)

func TestNormalizeCORS(t *testing.T) {
	for in, want := range map[string]string{
		"http://192.168.1.50:4200/":    "http://192.168.1.50:4200",
		"HTTP://Dashboard.EXAMPLE.lan": "http://dashboard.example.lan",
		" https://x.example:8443 ":     "https://x.example:8443",
	} {
		if got, err := NormalizeCORSOrigin(in); err != nil || got != want {
			t.Errorf("origin %q = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "192.168.1.50", "http://x/path", "http://u@x", "*", "null"} {
		if _, err := NormalizeCORSOrigin(bad); err == nil {
			t.Errorf("origin %q accepted", bad)
		}
	}
	for in, want := range map[string]string{"192.168.1.0/24": "192.168.1.0/24", "192.168.1.7/24": "192.168.1.0/24", "10.0.0.5": "10.0.0.5/32", "fd7a::1": "fd7a::1/128"} {
		if got, err := NormalizeCORSNetwork(in); err != nil || got != want {
			t.Errorf("network %q = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nope", "300.1.1.1/8", "10.0.0.0/40"} {
		if _, err := NormalizeCORSNetwork(bad); err == nil {
			t.Errorf("network %q accepted", bad)
		}
	}
}

func TestChangeCORSAddsRemovesAndKeepsTheRest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Save(File{OllamaHost: "http://h:1"}); err != nil {
		t.Fatal(err)
	}
	if v, ch, err := ChangeCORS(false, false, "http://a.example:1/"); err != nil || !ch || v != "http://a.example:1" {
		t.Fatalf("add: %q %v %v", v, ch, err)
	}
	if _, ch, _ := ChangeCORS(false, false, "http://a.example:1"); ch {
		t.Fatal("adding it twice changed the file")
	}
	if _, ch, err := ChangeCORS(true, false, "192.168.1.9/24"); err != nil || !ch {
		t.Fatalf("network: %v %v", ch, err)
	}
	o, n := CORS()
	if len(o) != 1 || len(n) != 1 || n[0] != "192.168.1.0/24" || Load().OllamaHost != "http://h:1" {
		t.Fatalf("%v %v %+v", o, n, Load())
	}
	if _, ch, _ := ChangeCORS(false, true, "http://a.example:1"); !ch {
		t.Fatal("remove did not change")
	}
	if _, ch, _ := ChangeCORS(false, true, "http://a.example:1"); ch {
		t.Fatal("removing a missing one changed")
	}
	if _, _, err := ChangeCORS(false, false, "nope"); err == nil {
		t.Fatal("a bad origin was saved")
	}
	// a file that cannot be parsed is never overwritten
	os.WriteFile(Path(), []byte("{broken"), 0o600)
	if _, _, err := ChangeCORS(false, false, "http://b.example:1"); err == nil {
		t.Fatal("changed a broken file")
	}
	if b, _ := os.ReadFile(Path()); string(b) != "{broken" {
		t.Fatalf("the broken file was overwritten: %q", b)
	}
}
