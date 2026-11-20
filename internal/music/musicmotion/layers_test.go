package musicmotion

import (
	"encoding/json"
	"strings"
	"testing"
)

// layerScript: two pieces of 10 s; a strip stays across both (0 s to 20 s) and a toast shows from 12 s to 16 s.
func layersScript() Script {
	s := testScript()
	s.Layers = []Layer{
		{ID: "years", Items: []LayerItem{{Part: "timeline-strip", Start: 0, End: 20}}},
		{ID: "notice", Items: []LayerItem{{Part: "achievement-toast", Start: 12, End: 16}}},
	}
	return s
}

func TestAScriptWithLayersIsValidAndRoundTrips(t *testing.T) {
	s := layersScript()
	if err := s.Validate(Catalog{Builtin: BuiltinParts()}, testMap(), good()); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	var back Script
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Layers) != 2 || back.Layers[1].Items[0].Part != "achievement-toast" || back.Layers[0].Items[0].End != 20 {
		t.Fatalf("%s", raw)
	}
	plain, _ := json.Marshal(testScript())
	if strings.Contains(string(plain), "layers") {
		t.Fatal("a script without layers must not write the field")
	}
}

func TestEachBadLayerIsRefusedAndNamed(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	cases := []struct {
		name string
		edit func(*Script)
		want string
	}{
		{"unknown part", func(s *Script) { s.Layers[0].Items[0].Part = "hologram" }, `layer "years" item #1: the part "hologram"`},
		{"parameter out of limits", func(s *Script) { s.Layers[0].Items[0].Params = map[string]any{"progress": 9999.0} }, `layer "years" item #1`},
		{"empty range", func(s *Script) { s.Layers[1].Items[0].End = 12 }, "ends before it starts"},
		{"outside the song", func(s *Script) { s.Layers[1].Items[0].End = 25 }, "outside the song"},
		{"before the song", func(s *Script) { s.Layers[1].Items[0].Start = -2 }, "outside the song"},
		{"overlap in a lane", func(s *Script) {
			s.Layers[0].Items = []LayerItem{{Part: "timeline-strip", Start: 0, End: 10}, {Part: "toggle", Start: 9, End: 15}}
		}, `layer "years" item #2 overlaps item #1`},
		{"repeated layer id", func(s *Script) { s.Layers[1].ID = "years" }, "repeated"},
		{"bad layer id", func(s *Script) { s.Layers[1].ID = "Not Ok" }, "the id must be"},
		{"too many layers", func(s *Script) {
			for i := 0; i < 3; i++ {
				s.Layers = append(s.Layers, Layer{ID: "l" + string(rune('a'+i))})
			}
		}, "up to 4 layers"},
		{"too many items", func(s *Script) {
			s.Layers[0].Items = nil
			for i := 0; i < 33; i++ {
				s.Layers[0].Items = append(s.Layers[0].Items, LayerItem{Part: "blank", Start: float64(i) * 0.5, End: float64(i)*0.5 + 0.4})
			}
		}, "up to 32 items"},
		{"the part of the piece twice", func(s *Script) { s.Layers[1].Items[0].Part = "stamp" }, `piece "p2" (#2): the part "stamp" is drawn twice (the main part and layer "notice" item #1)`},
		{"the part twice in two lanes", func(s *Script) { s.Layers[1].Items[0].Part = "timeline-strip" }, `the part "timeline-strip" is drawn twice`},
		{"an animation that cannot move", func(s *Script) {
			s.Layers[0].Items[0].Animate = Animate{"labels": {{T: 0, Value: "x"}}}
		}, `layer "years" item #1: the parameter "labels" of "timeline-strip" cannot be animated`},
		{"a keyframe after the item", func(s *Script) {
			s.Layers[0].Items[0].Animate = Animate{"progress": {{T: 0, Value: 0.0}, {T: 25, Value: 100.0}}}
		}, "outside the piece"},
	}
	for _, c := range cases {
		s := layersScript()
		c.edit(&s)
		err := s.Validate(cat, testMap(), good())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

func TestAPageCannotDrawMoreThanEightParts(t *testing.T) {
	s := testScript()
	s.Pieces[0].With = []PartRef{{Part: "glass-card"}, {Part: "flat-slabs"}, {Part: "counter"}}
	names := []string{"timeline-strip", "achievement-toast", "password-field", "particles"}
	for i, n := range names {
		s.Layers = append(s.Layers, Layer{ID: "l" + string(rune('a'+i)), Items: []LayerItem{{Part: n, Start: 0, End: 10}}})
	}
	// 1 + 3 + 4 = 8 is allowed; a second item in the first lane makes a ninth part on the first page
	if err := s.Validate(Catalog{Builtin: BuiltinParts()}, testMap(), good()); err != nil {
		t.Fatal(err)
	}
	s.Layers[0].Items = []LayerItem{{Part: "timeline-strip", Start: 0, End: 5}, {Part: "terminal", Start: 5, End: 10}}
	if err := s.Validate(Catalog{Builtin: BuiltinParts()}, testMap(), good()); err == nil || !strings.Contains(err.Error(), "the most is 8") {
		t.Fatalf("%v", err)
	}
}

func TestApplyLevelLeavesTheLayersAlone(t *testing.T) {
	s := layersScript()
	before, _ := json.Marshal(s.Layers)
	if _, err := ApplyLevel(&s, Catalog{Builtin: BuiltinParts()}, LevelSimple); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s.Layers)
	if string(before) != string(after) {
		t.Fatalf("%s != %s", before, after)
	}
}

func TestOnlyThePiecesAnItemTouchesDrawIt(t *testing.T) {
	s := layersScript()
	spec := ProjectSpec{Name: "x", Map: testMap(), Design: good(), Script: s, Cat: Catalog{Builtin: BuiltinParts()}, Lib: Library{}, FPS: 30, Width: 1280, Height: 720}
	pages, err := BuildPages(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pages[0].HTML, "layer-0") || strings.Contains(pages[0].HTML, "layer-1") {
		t.Fatal("the first piece (0 s to 10 s) draws the strip and not the notice (12 s to 16 s)")
	}
	if !strings.Contains(pages[1].HTML, "layer-0") || !strings.Contains(pages[1].HTML, "layer-1") {
		t.Fatal("the second piece draws both")
	}
	plain := testScript()
	spec.Script = plain
	old, _ := BuildPages(spec, nil)
	if strings.Contains(old[0].HTML, "__layers") || strings.Contains(old[0].HTML, "layer-") {
		t.Fatal("a page with no layers must not carry anything of them")
	}
}

func TestTheEstimateCountsTheLayerParts(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	without, _ := raw(testScript(), cat.GetMany(testScript().PartNames()), 30, 1280, 720)
	with, _ := raw(layersScript(), cat.GetMany(layersScript().PartNames()), 30, 1280, 720)
	if with <= without {
		t.Fatalf("%v should be more than %v", with, without)
	}
}

func tickPart() Part {
	return Part{
		Manifest: Manifest{Name: "tick", Description: "shows its time", Version: 1, Levels: []string{LevelSimple},
			Params: []Param{{Name: "n", Type: "int", Default: int64(0), Animatable: true}}},
		Fragment: `<div id="tk" style="position:absolute;left:10px;top:10px;font-size:40px"></div>
<script>var P=window.__params;window.__partSeek=function(t,b){document.getElementById('tk').textContent=t.toFixed(2)+'|'+P.n;};</script>`,
		Source: "builtin",
	}
}

// A piece that starts at 10 s of the song, with an item from 4 s to 14 s (a piece before it showed it from its start):
// the item's own time is the song time minus 4, its keyframes are read in that time, and it is hidden after 14 s.
func TestALayerItemContinuesAcrossACutAndHidesOutsideItsRange(t *testing.T) {
	sc := testScene()
	sc.SongStart, sc.Duration = 10, 10
	sc.Lines, sc.Hit = nil, nil
	tp := tickPart()
	params, _ := tp.ResolveParams(nil)
	sc.Layers = []SceneLayer{{Part: tp, Params: params, From: 4, To: 14, Animate: Animate{"n": {{T: 0, Value: 0.0}, {T: 10, Value: 100.0}}}}}
	s, _ := openPage(t, sc)
	var shown []string
	read := `(function(){var e=document.getElementById('layer-0');return [getComputedStyle(e).display, document.getElementById('tk').textContent];})()`
	evalAt(t, s, 0, read, &shown)
	if len(shown) != 2 || shown[0] != "contents" || shown[1] != "6.00|60" {
		t.Fatalf("at the start of the piece (song 10 s) the item is at 6 s of its own and n is 60: %v", shown)
	}
	evalAt(t, s, 3.5, read, &shown)
	if shown[0] != "contents" || shown[1] != "9.50|95" {
		t.Fatalf("%v", shown)
	}
	evalAt(t, s, 5, read, &shown)
	if shown[0] != "none" {
		t.Fatalf("at song 15 s the item is over: %v", shown)
	}
	// a piece that ends before the item starts has no page for it, and one that starts before shows it from its own start
	sc.SongStart, sc.Duration = 0, 10
	s2, _ := openPage(t, sc)
	evalAt(t, s2, 2, read, &shown)
	if shown[0] != "none" {
		t.Fatalf("at song 2 s the item (from 4 s) is not there yet: %v", shown)
	}
	evalAt(t, s2, 5, read, &shown)
	if shown[0] != "contents" || shown[1] != "1.00|10" {
		t.Fatalf("at song 5 s it has been there for 1 s: %v", shown)
	}
}
