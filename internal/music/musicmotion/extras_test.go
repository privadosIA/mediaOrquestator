package musicmotion

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func scenePart(t *testing.T, name string, params map[string]any) ScenePart {
	t.Helper()
	p := mustPart(t, name)
	resolved, err := p.ResolveParams(params)
	if err != nil {
		t.Fatal(err)
	}
	return ScenePart{Part: p, Params: resolved}
}

func TestAPieceWithAnExtraPartDrawsBothEachWithItsOwnParametersOnTheBeat(t *testing.T) {
	main := scenePart(t, "counter", map[string]any{"from": 7, "step": 100, "unit": "beats"})
	extra := scenePart(t, "achievement-toast", map[string]any{"title": "HELLO ERA", "beats": 8})
	sc := Scene{Part: main.Part, Params: main.Params, Extras: []ScenePart{extra}, Design: good(), Mode: ModeDark, Duration: 6, Width: 1280, Height: 720}
	s, _ := openPage(t, sc)
	text := func(at float64, expr string) string {
		var v string
		evalAt(t, s, at, expr, &v)
		return v
	}
	if text(1.5, `document.querySelector('#at .t').textContent`) != "HELLO ERA" {
		t.Fatal("the extra part has its own parameters")
	}
	if got := text(0.1, `document.querySelector('#ct .u').textContent`); got != "beats" {
		t.Fatalf("the main part keeps its own: %q", got)
	}
	a, b := text(1.0, `document.querySelector('#ct .n').textContent`), text(1.5, `document.querySelector('#ct .n').textContent`)
	if a != "107" || b != "207" {
		t.Fatalf("the main part moves on the beat from its own value (7 plus 100 a beat): %q %q", a, b)
	}
	if evalNumber(t, s, 0, `parseFloat(document.getElementById('at').style.opacity)`) > 0.05 || evalNumber(t, s, 1.5, `parseFloat(document.getElementById('at').style.opacity)`) < 0.9 {
		t.Fatal("the extra part moves on the beat too: out before its first beat, in later")
	}
}

func TestExtraPartsAreCheckedLikeTheMainOne(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	mk := func(with ...PartRef) Script {
		s := testScript()
		s.Pieces[0].With = with
		return s
	}
	cases := []struct {
		name string
		with []PartRef
		want string
	}{
		{"an extra part that is not there", []PartRef{{Part: "hologram"}}, "hologram"},
		{"a parameter outside its limits", []PartRef{{Part: "counter", Params: map[string]any{"step": 99999.0}}}, `"step"`},
		{"more than three", []PartRef{{Part: "counter"}, {Part: "terminal"}, {Part: "stamp"}, {Part: "sunburst"}}, "up to 3"},
		{"the main part again", []PartRef{{Part: "toggle"}}, "twice"},
		{"the same extra twice", []PartRef{{Part: "counter"}, {Part: "counter"}}, "twice"},
	}
	for _, c := range cases {
		err := mk(c.with...).Validate(cat, testMap(), good())
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), `"p1"`) {
			t.Errorf("%s: %v, want an error naming the piece and %q", c.name, err, c.want)
		}
	}
	if err := mk(PartRef{Part: "counter"}, PartRef{Part: "stamp"}).Validate(cat, testMap(), good()); err != nil {
		t.Fatalf("two different extras are fine: %v", err)
	}
}

func TestTwoFragmentsOfOnePartAreRefusedWhenThePageIsBuilt(t *testing.T) {
	main := scenePart(t, "counter", nil)
	sc := Scene{Part: main.Part, Params: main.Params, Extras: []ScenePart{main}, Design: good(), Mode: ModeDark, Duration: 4, Width: 1280, Height: 720}
	if _, err := BuildPage(sc); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("%v", err)
	}
	sc.Extras = []ScenePart{scenePart(t, "stamp", nil), scenePart(t, "sunburst", nil), scenePart(t, "toggle", nil), scenePart(t, "terminal", nil)}
	if _, err := BuildPage(sc); err == nil || !strings.Contains(err.Error(), "up to 3") {
		t.Fatalf("%v", err)
	}
}

func TestAProjectIsWrittenWithTheExtraPartsOfItsPieces(t *testing.T) {
	dir := t.TempDir()
	spec := shortSpec(t, dir+"/proj", makeSongOrFake(t, dir))
	extra := "achievement-toast"
	if spec.Script.Pieces[0].Part == extra {
		extra = "sunburst"
	}
	spec.Script.Pieces[0].With = []PartRef{{Part: extra, Params: map[string]any{}}}
	if _, err := WriteProject(spec); err != nil {
		t.Fatal(err)
	}
	html := readFile(t, dir+"/proj/motion/p1.html")
	if strings.Count(html, "__partSeeks") < 4 { // the collector of each fragment: one for the main part and one for the extra
		t.Fatal("the page of the piece carries its extra part")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAPageKnowsWhereItsPieceSitsInTheSong(t *testing.T) {
	main := scenePart(t, "counter", nil)
	sc := Scene{Part: main.Part, Params: main.Params, Design: good(), Mode: ModeDark, Duration: 4, Width: 1280, Height: 720, SongStart: 40, SongLength: 200}
	s, _ := openPage(t, sc)
	if evalNumber(t, s, 1, `window.__scene.song_start`) != 40 || evalNumber(t, s, 1, `window.__scene.song_length`) != 200 {
		t.Fatal("a piece that starts at 40 s of a 200 s song gives a page that reads them")
	}
	none := Scene{Part: main.Part, Params: main.Params, Design: good(), Mode: ModeDark, Duration: 4, Width: 1280, Height: 720}
	s2, _ := openPage(t, none)
	if evalNumber(t, s2, 1, `window.__scene.song_start`) != 0 || evalNumber(t, s2, 1, `window.__scene.song_length`) != 0 {
		t.Fatal("a scene with no song reads zeros")
	}
}

func TestAProjectWritesEachPiecesPlaceInTheSong(t *testing.T) {
	dir := t.TempDir()
	spec := shortSpec(t, dir+"/proj", makeSongOrFake(t, dir))
	if _, err := WriteProject(spec); err != nil {
		t.Fatal(err)
	}
	html := readFile(t, dir+"/proj/motion/p2.html")
	second := spec.Script.Pieces[1].Start
	if !strings.Contains(html, `"song_length":8`) || !strings.Contains(html, `"song_start":`+trimFloat(second)) {
		t.Fatalf("the page of the second piece knows it starts at %v of an 8 s song", second)
	}
}

func trimFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 3, 64), "0"), ".")
}

// --- a design of its own for a piece --------------------------------------------------------------------

func TestTwoPiecesOfOneScriptAreDrawnWithDifferentAccents(t *testing.T) {
	dir := t.TempDir()
	spec := shortSpec(t, dir+"/proj", makeSongOrFake(t, dir))
	spec.Script.Pieces[0].Design = &DesignOverride{Accent: "#ffb000", Dark: "#1a1000", Light: "#fff4d6"}
	if _, err := WriteProject(spec); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, dir+"/proj/motion/p1.html")
	second := readFile(t, dir+"/proj/motion/p2.html")
	if !strings.Contains(first, "--accent:#ffb000") && !strings.Contains(first, "#ffb000") {
		t.Fatal("the first piece is drawn with its own accent")
	}
	if strings.Contains(second, "#ffb000") || !strings.Contains(second, "#2bff88") {
		t.Fatal("the next piece is drawn with the song's design")
	}
	if spec.Design.Accent != "#2bff88" || spec.Design.Dark != "#0b0f14" {
		t.Fatal("the song's design does not change")
	}
}

func TestAnOverrideThatMakesTheDesignInvalidIsRefusedNamingThePieceAndTheField(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	cases := map[string]*DesignOverride{
		"an accent that is not a colour": {Accent: "amber"},
		"an accent with no contrast":     {Accent: "#101418"},
		"a font that is not there":       {Display: "comic"},
		"a verse style that is not":      {VerseStyle: "wiggle"},
	}
	for name, o := range cases {
		s := testScript()
		s.Pieces[1].Design = o
		err := s.Validate(cat, testMap(), good())
		if err == nil || !strings.Contains(err.Error(), `"p2"`) || !strings.Contains(err.Error(), "design.") {
			t.Errorf("%s: %v, want an error naming the piece and a design field", name, err)
		}
	}
	s := testScript()
	s.Pieces[1].Design = &DesignOverride{Accent: "#ffb000", Dark: "#1a1000", Light: "#fff4d6", VerseStyle: "highlight"}
	if err := s.Validate(cat, testMap(), good()); err != nil {
		t.Fatalf("a valid override: %v", err)
	}
}

func TestAPieceWithNoDesignIsBuiltAsBefore(t *testing.T) {
	d := good()
	same := d.With(nil)
	if !reflect.DeepEqual(d, same) {
		t.Fatal("no override gives the design as it is")
	}
	same.Modes["chorus"] = ModeLight
	if d.Modes["chorus"] != ModeDark {
		t.Fatal("the merged design does not share its map with the song's")
	}
	empty := d.With(&DesignOverride{})
	if !reflect.DeepEqual(d, empty) {
		t.Fatal("an override with nothing set changes nothing")
	}
}
