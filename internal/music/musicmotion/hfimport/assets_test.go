package hfimport

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const strokeSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 5 L10 5" stroke="#fff"/></svg>`

// pngBytes makes a real PNG of about n bytes (noise does not compress).
func pngBytes(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	rng := rand.New(rand.NewSource(1))
	for i := range img.Pix {
		img.Pix[i] = byte(rng.Intn(256))
	}
	for x := 0; x < side; x++ {
		img.Set(x, 0, color.NRGBA{255, 0, 0, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func files(m map[string][]byte) Source {
	return func(rel string) ([]byte, error) {
		if b, ok := m[rel]; ok {
			return b, nil
		}
		return nil, errors.New("not there")
	}
}

func withBody(extraCSS, extraBody string) string {
	b := strings.Replace(kineticBlock, `<div id="k-sub"`, extraBody+`<div id="k-sub"`, 1)
	return strings.Replace(b, `</style>`, extraCSS+"</style>", 1)
}

func convertWith(t *testing.T, block string, m map[string][]byte) Result {
	t.Helper()
	return ConvertWith("pic", []byte(block), nil, files(m))
}

func TestAFontFileUnderAssetsNoLongerRefusesTheBlock(t *testing.T) {
	block := strings.Replace(kineticBlock, `url(fonts/archivo.woff2)`, `url('assets/fonts/archivo.woff2')`, 1)
	r := Convert("font", []byte(block), nil)
	if len(r.Problems) > 0 {
		t.Fatalf("a font nothing reads must not refuse the block: %v", r.Problems)
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "@font-face dropped") {
		t.Errorf("the warning says the fonts are the design's: %v", r.Warnings)
	}
	// outside @font-face it is still a file nothing carries
	r = Convert("font", []byte(withBody(`.x{background:url(assets/fonts/a.woff2)}`, "")), nil)
	if !strings.Contains(strings.Join(r.Problems, " "), "assets/fonts/a.woff2") {
		t.Errorf("a font file used as a picture is refused: %v", r.Problems)
	}
}

func TestAnInkStrokeSVGIsEmbeddedAndListed(t *testing.T) {
	r := convertWith(t, withBody("", `<img class="ink" src="assets/stroke.svg">`), map[string][]byte{"assets/stroke.svg": []byte(strokeSVG)})
	if len(r.Problems) > 0 {
		t.Fatalf("%v", r.Problems)
	}
	want := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(strokeSVG))
	if !strings.Contains(r.Fragment, `src="`+want+`"`) || strings.Contains(r.Fragment, "assets/stroke.svg") {
		t.Fatalf("the <img> must carry the data URL: %s", r.Fragment)
	}
	if len(r.Pictures) != 1 || r.Pictures[0].Name != "stroke.svg" || r.Pictures[0].Kind != "svg" || r.Pictures[0].Bytes != len(strokeSVG) || r.Manifest.Pictures != 1 {
		t.Errorf("pictures: %+v manifest %d", r.Pictures, r.Manifest.Pictures)
	}
	if strings.Contains(strings.Join(r.Warnings, " "), "own colours") {
		t.Errorf("a drawing does not warn about colours: %v", r.Warnings)
	}
}

func TestARasterBackgroundIsEmbeddedAndWarns(t *testing.T) {
	data := pngBytes(t, 120)
	r := convertWith(t, withBody(`.k-wrap{background-image:url(assets/paper.png)}`, ""), map[string][]byte{"assets/paper.png": data})
	if len(r.Problems) > 0 {
		t.Fatalf("%v", r.Problems)
	}
	if !strings.Contains(r.Fragment, "url(\"data:image/png;base64,") || strings.Contains(r.Fragment, "assets/paper.png") {
		t.Errorf("the url() must be replaced in place: %s", r.Fragment[:300])
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "keeps its own colours") {
		t.Errorf("the warning about colours: %v", r.Warnings)
	}
}

func TestPicturesTooHeavyMissingOrOutsideAreRefusedNamingThem(t *testing.T) {
	big := bytes.Repeat([]byte{1}, 300<<10)
	r := convertWith(t, withBody("", `<img src="assets/hero.png">`), map[string][]byte{"assets/hero.png": append(pngBytes(t, 8)[:8:8], big...)})
	if !strings.Contains(strings.Join(r.Problems, " "), "assets/hero.png is 30") {
		t.Errorf("too heavy: %v", r.Problems)
	}
	r = convertWith(t, withBody("", `<img src="assets/nothing.png">`), nil)
	if !strings.Contains(strings.Join(r.Problems, " "), "assets/nothing.png is not there") {
		t.Errorf("missing: %v", r.Problems)
	}
	r = convertWith(t, withBody("", `<img src="assets/../secret.png">`), map[string][]byte{"secret.png": pngBytes(t, 8)})
	if !strings.Contains(strings.Join(r.Problems, " "), "not a clean path") {
		t.Errorf("a path out of assets/: %v", r.Problems)
	}
	r = convertWith(t, withBody("", `<img src="assets/liar.png">`), map[string][]byte{"assets/liar.png": []byte("not a picture")})
	if !strings.Contains(strings.Join(r.Problems, " "), "is not a PNG picture") {
		t.Errorf("a file that is not what its name says: %v", r.Problems)
	}
	r = convertWith(t, withBody("", `<img src="assets/bad.svg">`), map[string][]byte{"assets/bad.svg": []byte(`<svg><script>alert(1)</script></svg>`)})
	if !strings.Contains(strings.Join(r.Problems, " "), "assets/bad.svg: it has a script") {
		t.Errorf("an unsafe drawing: %v", r.Problems)
	}
	r = convertWith(t, withBody("", `<img src="assets/ref.svg">`), map[string][]byte{"assets/ref.svg": []byte(`<svg><image href="other.png"/></svg>`)})
	if !strings.Contains(strings.Join(r.Problems, " "), "refers to another file") {
		t.Errorf("a drawing that points at another file: %v", r.Problems)
	}
}

func TestThePicturesOfAPartHaveAWeightLimitAndTheSameFileCountsTwice(t *testing.T) {
	data := pngBytes(t, 120) // ~57 KB of noise
	m := map[string][]byte{"assets/n.png": data}
	var imgs strings.Builder
	for i := 0; i < 20; i++ {
		imgs.WriteString(`<img src="assets/n.png">`)
	}
	r := convertWith(t, withBody("", imgs.String()), m)
	if !strings.Contains(strings.Join(r.Problems, " "), "once embedded") {
		t.Errorf("many copies must pass the 600 KB cap (file %d bytes): %v", len(data), r.Problems)
	}
	var few strings.Builder
	for i := 0; i < 2; i++ {
		few.WriteString(`<img src="assets/n.png">`)
	}
	r = convertWith(t, withBody("", few.String()), m)
	if len(r.Problems) > 0 || len(r.Pictures) != 1 || strings.Count(r.Fragment, "data:image/png") != 2 {
		t.Errorf("twice: embedded twice, listed once: %v %+v", r.Problems, r.Pictures)
	}
}

func TestMediaAndRuntimesInAssetsAreStillRefused(t *testing.T) {
	for _, f := range []string{"loop.gif", "anim.json", "model.glb", "clip.mp4", "song.mp3"} {
		r := convertWith(t, withBody("", `<img src="assets/`+f+`">`), map[string][]byte{"assets/" + f: {1, 2, 3}})
		if len(r.Problems) == 0 || !strings.Contains(strings.Join(r.Problems, " "), "assets/"+f) {
			t.Errorf("%s must be refused naming it: %v", f, r.Problems)
		}
	}
	// an <img> that is not an asset, a srcset and a poster still refuse; so does a data-var-src
	for _, in := range []string{`<img src="https://x.example/a.png">`, `<img src="assets/a.png" srcset="b.png 2x">`, `<img>`} {
		r := convertWith(t, withBody("", in), map[string][]byte{"assets/a.png": pngBytes(t, 8)})
		if len(r.Problems) == 0 {
			t.Errorf("%q must be refused", in)
		}
	}
}

func TestImportReadsPicturesFromTheBlocksFolderThenFromTheSharedAssets(t *testing.T) {
	dir := t.TempDir()
	comp := filepath.Join(dir, "compositions")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(comp, "own", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(dir, "assets", "shared.svg"), strokeSVG)
	must(filepath.Join(comp, "own", "assets", "mine.svg"), strokeSVG+"<!-- own -->")
	must(filepath.Join(comp, "own", "own.html"), withBody("", `<img src="assets/mine.svg"><img src="assets/shared.svg">`))
	files, err := find(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("%v %v", files, err)
	}
	r := ConvertWith(files[0].name, mustRead(t, files[0].path), nil, sourceFor(dir, files[0]))
	if len(r.Problems) > 0 || len(r.Pictures) != 2 {
		t.Fatalf("both files are found, the block's own folder first: %v %+v", r.Problems, r.Pictures)
	}
	src := sourceFor(dir, files[0])
	for _, bad := range []string{"../x.svg", "/etc/passwd", "assets/../../x"} {
		if _, err := src(bad); err == nil {
			t.Errorf("%q must not be read", bad)
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
