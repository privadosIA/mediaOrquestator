package settings

import "testing"

func TestValidateDiffRhythm2Address(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.50":             "http://192.168.1.50:8788",
		"192.168.1.50:9000":        "http://192.168.1.50:9000",
		"http://musica.local":      "http://musica.local:8788",
		"https://musica.local:443": "https://musica.local:443",
		" Musica.Local/ ":          "http://musica.local:8788",
		"[::1]":                    "http://[::1]:8788",
	} {
		got, err := ValidateDiffRhythm2Address(in)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://x", "http://u:p@x", "http://x/path", "http://x:99999"} {
		if _, err := ValidateDiffRhythm2Address(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestDiffRhythm2Settings_SavedPrivatelyAndConsentTiedToTheAddress(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if h, tok := DiffRhythm2(); h != "" || tok != "" {
		t.Fatalf("nothing saved yet: %q %q", h, tok)
	}
	if err := AcknowledgeDiffRhythm2(); err == nil {
		t.Error("there is nothing to accept without an address")
	}
	if err := SetDiffRhythm2("http://10.0.0.5:8788", "secret"); err != nil {
		t.Fatal(err)
	}
	if h, tok := DiffRhythm2(); h != "http://10.0.0.5:8788" || tok != "secret" {
		t.Errorf("saved = %q %q", h, tok)
	}
	if DiffRhythm2Acknowledged("http://10.0.0.5:8788") {
		t.Error("not accepted yet")
	}
	if err := AcknowledgeDiffRhythm2(); err != nil {
		t.Fatal(err)
	}
	if !DiffRhythm2Acknowledged("http://10.0.0.5:8788") {
		t.Error("accepted for that address")
	}
	// Another address needs its own acceptance.
	if err := SetDiffRhythm2("http://10.0.0.6:8788", "secret"); err != nil {
		t.Fatal(err)
	}
	if DiffRhythm2Acknowledged("http://10.0.0.6:8788") || DiffRhythm2Acknowledged("http://10.0.0.5:8788") {
		t.Error("a changed address loses the acceptance")
	}
	// Keeping the same address keeps it.
	if err := AcknowledgeDiffRhythm2(); err != nil {
		t.Fatal(err)
	}
	if err := SetDiffRhythm2("http://10.0.0.6:8788", "other"); err != nil {
		t.Fatal(err)
	}
	if !DiffRhythm2Acknowledged("http://10.0.0.6:8788") {
		t.Error("the same address keeps its acceptance")
	}
	// Removing it clears everything, and an empty file is not left behind.
	if err := SetDiffRhythm2("", "ignored"); err != nil {
		t.Fatal(err)
	}
	if h, tok := DiffRhythm2(); h != "" || tok != "" {
		t.Errorf("removed: %q %q", h, tok)
	}
	if DiffRhythm2Acknowledged("http://10.0.0.6:8788") {
		t.Error("removed with the address")
	}
}
