package blender

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tdewolff/font"
)

// reference is what an independent decoder (fontTools 4.68, which reads the WOFF2 directly) says of each file of the pack:
// the number of glyphs and the advance of "Hola" in font units. The pure-Go decoder must give the same for all of them,
// which is what the choice of decoder in extend-blender-scene-build rests on (no download of TTFs needed).
var reference = map[string]struct{ glyphs, hola int }{
	"bebas-neue-400":       {269, 1565},
	"fraunces-400":         {245, 4512},
	"fraunces-700":         {245, 4797},
	"inter-400":            {518, 4396},
	"inter-700":            {518, 4530},
	"jetbrains-mono-400":   {394, 2400},
	"jetbrains-mono-700":   {394, 2400},
	"playfair-display-400": {318, 2089},
	"playfair-display-700": {318, 2194},
	"space-grotesk-400":    {291, 2094},
	"space-grotesk-700":    {291, 2112},
}

func TestEnsureTTF_EveryFileOfThePackDecodesLikeTheReferenceDecoder(t *testing.T) {
	files, err := filepath.Glob("../../music/musicmotion/fonts/*.woff2")
	if err != nil || len(files) != len(reference) {
		t.Fatalf("the pack has %d files, the reference %d: %v", len(files), len(reference), err)
	}
	dir := t.TempDir()
	for _, f := range files {
		name := filepath.Base(f[:len(f)-len(".woff2")])
		want, ok := reference[name]
		if !ok {
			t.Errorf("%s has no reference", name)
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		path, err := EnsureTTF(dir, raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		data, _ := os.ReadFile(path)
		s, err := font.ParseSFNT(data, 0)
		if err != nil {
			t.Errorf("%s: the decoded font does not parse: %v", name, err)
			continue
		}
		adv := 0
		for _, r := range "Hola" {
			adv += int(s.GlyphAdvance(s.GlyphIndex(r)))
		}
		if int(s.NumGlyphs()) != want.glyphs || adv != want.hola {
			t.Errorf("%s: %d glyphs and an advance of %d for Hola, the reference says %d and %d", name, s.NumGlyphs(), adv, want.glyphs, want.hola)
		}
	}
}

func TestEnsureTTF_ADecodedFontIsReusedAndABrokenOneLeavesNothing(t *testing.T) {
	raw, err := os.ReadFile("../../music/musicmotion/fonts/inter-400.woff2")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "fonts", "deep") // the folder does not exist yet
	first, err := EnsureTTF(dir, raw)
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(dir, FontSHA(raw)+".ttf") {
		t.Errorf("the file is named by the SHA-256 of the WOFF2: %s", first)
	}
	// reused: a marker written into the file survives a second call, which therefore did not decode again
	if err := os.WriteFile(first, []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	if again, err := EnsureTTF(dir, raw); err != nil || again != first {
		t.Fatalf("again = %q %v", again, err)
	}
	if got, _ := os.ReadFile(first); string(got) != "marker" {
		t.Error("a font already decoded must be reused, not decoded again")
	}
	if _, err := EnsureTTF(dir, []byte("not a font at all")); err == nil {
		t.Error("garbage is not a font")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("a failed decode must leave nothing behind, the folder holds %d files", len(entries))
	}
}
