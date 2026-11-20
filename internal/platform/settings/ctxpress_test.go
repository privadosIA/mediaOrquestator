package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateCtx(t *testing.T) {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HOME", d)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, "cfg"))
	for _, k := range []string{"CTXPRESS_URL", "CTXPRESS_TOKEN", "CTXPRESS_MIN_CHARS"} {
		t.Setenv(k, "")
	}
}

func TestCtxpressIsOffWithNothingAndFollowsTheEnvironmentWhenNothingIsSaved(t *testing.T) {
	isolateCtx(t)
	if c := Ctxpress(); c.Enabled || c.Source != "none" || c.MinChars != DefaultCtxpressMinChars {
		t.Fatalf("%+v", c)
	}
	t.Setenv("CTXPRESS_URL", "http://127.0.0.1:8080/")
	t.Setenv("CTXPRESS_TOKEN", "envtoken")
	t.Setenv("CTXPRESS_MIN_CHARS", "500")
	c := Ctxpress()
	if !c.Enabled || c.URL != "http://127.0.0.1:8080" || c.Token != "envtoken" || c.MinChars != 500 || c.Source != "env" {
		t.Fatalf("%+v", c)
	}
}

func TestSavedCtxpressSettingsBeatTheEnvironmentAndTheTokenLivesInTheKeyStore(t *testing.T) {
	isolateCtx(t)
	t.Setenv("CTXPRESS_URL", "http://env.example:1")
	t.Setenv("CTXPRESS_TOKEN", "envtoken")
	if err := SaveCtxpress(&CtxpressSettings{Enabled: true, URL: "http://127.0.0.1:8080", MinChars: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := SetCtxpressToken("the-saved-token"); err != nil {
		t.Fatal(err)
	}
	c := Ctxpress()
	if !c.Enabled || c.URL != "http://127.0.0.1:8080" || c.MinChars != 3000 || c.Token != "the-saved-token" || c.Source != "setting" {
		t.Fatalf("%+v", c)
	}
	// the token is never in settings.json
	raw, _ := os.ReadFile(Path())
	if strings.Contains(string(raw), "the-saved-token") {
		t.Fatal("the token was written to settings.json")
	}
	if k, _ := os.ReadFile(KeysPath()); !strings.Contains(string(k), "the-saved-token") {
		t.Fatal("the token is in the key store")
	}
	// saved but off: off, even with the environment set
	if err := SaveCtxpress(&CtxpressSettings{Enabled: false, URL: "http://127.0.0.1:8080"}); err != nil {
		t.Fatal(err)
	}
	if Ctxpress().Enabled {
		t.Fatal("a saved off beats the environment")
	}
	// removing the section gives the environment back; removing the token falls back to the environment's
	if err := SaveCtxpress(nil); err != nil {
		t.Fatal(err)
	}
	if err := SetCtxpressToken(""); err != nil {
		t.Fatal(err)
	}
	if c := Ctxpress(); c.Source != "env" || !c.Enabled || c.Token != "envtoken" {
		t.Fatalf("%+v", c)
	}
}

func TestCtxpressValidationAndWarning(t *testing.T) {
	isolateCtx(t)
	if err := SaveCtxpress(&CtxpressSettings{Enabled: true, URL: "ftp://x"}); err == nil {
		t.Error("only http and https")
	}
	if err := SaveCtxpress(&CtxpressSettings{Enabled: true, URL: "http://127.0.0.1:8080", MinChars: 5}); err == nil {
		t.Error("too small a min_chars")
	}
	if w := CtxpressWarning("http://127.0.0.1:8080"); w != "" {
		t.Errorf("local plain http is fine: %q", w)
	}
	if w := CtxpressWarning("http://10.20.30.40:8080"); !strings.Contains(w, "clear text") {
		t.Errorf("remote plain http warns: %q", w)
	}
	if w := CtxpressWarning("https://ctx.example.com"); w != "" {
		t.Errorf("https does not warn: %q", w)
	}
	if err := SetCtxpressToken(strings.Repeat("x", 501)); err == nil {
		t.Error("a token that is too long")
	}
}
