package settings

import "testing"

func TestValidateHeartMulaAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.50":             "http://192.168.1.50:8787",
		"192.168.1.50:9000":        "http://192.168.1.50:9000",
		"http://musica.local":      "http://musica.local:8787",
		"https://musica.local:443": "https://musica.local:443",
		" Musica.Local/ ":          "http://musica.local:8787",
		"[::1]":                    "http://[::1]:8787",
	} {
		got, err := ValidateHeartMulaAddress(in)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://x", "http://u:p@x", "http://x/path", "http://x:99999"} {
		if _, err := ValidateHeartMulaAddress(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestHeartMulaSettings_SavedPrivatelyAndConsentTiedToTheAddress(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if h, tok := HeartMula(); h != "" || tok != "" {
		t.Fatalf("nothing saved yet: %q %q", h, tok)
	}
	if err := AcknowledgeHeartMula(); err == nil {
		t.Error("there is nothing to accept without an address")
	}
	if err := SetHeartMula("http://10.0.0.5:8787", "secret"); err != nil {
		t.Fatal(err)
	}
	if h, tok := HeartMula(); h != "http://10.0.0.5:8787" || tok != "secret" {
		t.Errorf("saved = %q %q", h, tok)
	}
	if HeartMulaAcknowledged("http://10.0.0.5:8787") {
		t.Error("not accepted yet")
	}
	if err := AcknowledgeHeartMula(); err != nil {
		t.Fatal(err)
	}
	if !HeartMulaAcknowledged("http://10.0.0.5:8787") {
		t.Error("accepted for that address")
	}
	// Another address needs its own acceptance.
	if err := SetHeartMula("http://10.0.0.6:8787", "secret"); err != nil {
		t.Fatal(err)
	}
	if HeartMulaAcknowledged("http://10.0.0.6:8787") || HeartMulaAcknowledged("http://10.0.0.5:8787") {
		t.Error("a changed address loses the acceptance")
	}
	// Keeping the same address keeps it.
	if err := AcknowledgeHeartMula(); err != nil {
		t.Fatal(err)
	}
	if err := SetHeartMula("http://10.0.0.6:8787", "other"); err != nil {
		t.Fatal(err)
	}
	if !HeartMulaAcknowledged("http://10.0.0.6:8787") {
		t.Error("the same address keeps its acceptance")
	}
	// Removing it clears everything, and an empty file is not left behind.
	if err := SetHeartMula("", "ignored"); err != nil {
		t.Fatal(err)
	}
	if h, tok := HeartMula(); h != "" || tok != "" {
		t.Errorf("removed: %q %q", h, tok)
	}
	if HeartMulaAcknowledged("http://10.0.0.6:8787") {
		t.Error("removed with the address")
	}
}
