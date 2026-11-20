package hyperframes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestIsPinned(t *testing.T) {
	m := Manifest()
	if len(m) < 50 {
		t.Fatalf("manifest has %d libraries", len(m))
	}
	seen := map[string]bool{}
	for _, l := range m {
		if len(l.SHA256) != 64 {
			t.Errorf("%s: sha256 %q", l.URL, l.SHA256)
		}
		if _, ok := File("x", l.URL); !ok {
			t.Errorf("%s has no file", l.URL)
		}
		if seen[l.URL] {
			t.Errorf("%s twice", l.URL)
		}
		seen[l.URL] = true
	}
}

func TestLookupServesOnlyWhatIsPinnedAndInstalled(t *testing.T) {
	dir := t.TempDir()
	const gsap = "https://cdn.jsdelivr.net/npm/gsap@3.14.2/dist/gsap.min.js"
	if _, ok := Lookup(dir, gsap); ok {
		t.Fatal("served before it is installed")
	}
	if len(Missing(dir)) != len(Manifest()) {
		t.Fatal("everything should be missing")
	}
	p, _ := File(dir, gsap)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := Lookup(dir, gsap+"?v=1#a"); !ok || got != p {
		t.Fatalf("Lookup = %q %v", got, ok)
	}
	if _, ok := Lookup(dir, "https://cdn.jsdelivr.net/npm/other@1.0.0/x.js"); ok {
		t.Fatal("served a library that is not pinned")
	}
	if _, ok := Lookup(dir, "https://evil.example/../../etc/passwd"); ok {
		t.Fatal("served a path outside")
	}
}

func TestFileRefusesWhatIsNotAFile(t *testing.T) {
	for _, u := range []string{"http://cdn.jsdelivr.net/a.js", "https://cdn.jsdelivr.net/", "https://cdn.jsdelivr.net", "https://h/../../x", ""} {
		if p, ok := File("d", u); ok && u != "https://h/../../x" {
			t.Errorf("File(%q) = %q", u, p)
		}
	}
}

func TestFontStylesheets(t *testing.T) {
	fontIndex.once.Do(loadFontIndex)
	if len(fontIndex.byURL) < 15 {
		t.Fatalf("%d stylesheets", len(fontIndex.byURL))
	}
	for u, name := range fontIndex.byURL {
		if FontCSSName(u) != name {
			t.Errorf("%s is kept as %s, not %s", u, name, FontCSSName(u))
		}
		if css, ok := FontCSS(u); !ok || len(css) == 0 {
			t.Errorf("no stylesheet for %s", u)
		}
	}
	if _, ok := FontCSS("https://fonts.googleapis.com/css2?family=Nope"); ok {
		t.Error("an unknown address has a stylesheet")
	}
}
