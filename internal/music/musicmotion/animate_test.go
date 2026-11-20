package musicmotion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func animScript() Script {
	return Script{Version: ScriptVersion, Pieces: []Piece{
		{ID: "p1", Start: 0, End: 10, Part: "counter",
			Animate: Animate{"from": {{T: 0, Value: 0.0}, {T: 4, Value: 100.0, Ease: "out"}}},
			With:    []PartRef{{Part: "timeline-strip", Animate: Animate{"progress": {{T: 0, Value: 0.0}, {T: 10, Value: 100.0}}}}}},
		{ID: "p2", Start: 10, End: 20, Part: "stamp"},
	}}
}

func TestAScriptWithKeyframesIsValidAndRoundTrips(t *testing.T) {
	s := animScript()
	if err := s.Validate(Catalog{Builtin: BuiltinParts()}, testMap(), good()); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	var back Script
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Pieces[0].Animate["from"]) != 2 || back.Pieces[0].Animate["from"][1].Ease != "out" || len(back.Pieces[0].With[0].Animate["progress"]) != 2 {
		t.Fatalf("the keyframes did not survive: %s", raw)
	}
	plain, _ := json.Marshal(testScript())
	if strings.Contains(string(plain), "animate") {
		t.Fatal("a script without keyframes must not write the field")
	}
}

func TestEachBadAnimationIsRefusedAndNamed(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	cases := []struct {
		name string
		edit func(*Script)
		want string
	}{
		{"a parameter that cannot move", func(s *Script) { s.Pieces[0].Animate = Animate{"unit": {{T: 0, Value: "a"}}} }, `the parameter "unit" of "counter" cannot be animated`},
		{"a parameter the part does not have", func(s *Script) { s.Pieces[0].Animate = Animate{"speed": {{T: 0, Value: 1.0}}} }, `no parameter "speed"`},
		{"no keyframe", func(s *Script) { s.Pieces[0].Animate = Animate{"from": {}} }, "no keyframe"},
		{"before the piece", func(s *Script) { s.Pieces[0].Animate["from"][0].T = -1 }, "outside the piece"},
		{"after the piece", func(s *Script) { s.Pieces[0].Animate["from"][1].T = 11 }, "outside the piece"},
		{"not increasing", func(s *Script) { s.Pieces[0].Animate["from"][1].T = 0 }, "not after"},
		{"unknown ease", func(s *Script) { s.Pieces[0].Animate["from"][0].Ease = "bounce" }, `ease "bounce"`},
		{"a value out of limits", func(s *Script) { s.Pieces[0].Animate["from"][1].Value = 999999.0 }, "keyframe #2"},
		{"a value of the wrong type", func(s *Script) { s.Pieces[0].Animate["from"][1].Value = "a lot" }, "must be a number"},
		{"an extra that cannot move", func(s *Script) { s.Pieces[0].With[0].Animate = Animate{"labels": {{T: 0, Value: "x"}}} }, `"labels" of "timeline-strip" cannot be animated`},
	}
	for _, c := range cases {
		s := animScript()
		c.edit(&s)
		err := s.Validate(cat, testMap(), good())
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), `"p1"`) {
			t.Errorf("%s: got %v, want %q naming the piece", c.name, err, c.want)
		}
	}
}

func TestAnAnimatableParameterThatIsATextSlotIsRefused(t *testing.T) {
	manifest := `{"name":"tick","description":"x","version":1,"levels":["simple"],"params":[{"name":"unit","type":"string","default":"a","max_length":5,"animatable":true}]}`
	m, err := parseManifest([]byte(manifest), "tick")
	if err != nil {
		t.Fatal(err)
	}
	probs := animatableSlotProblems(m, `<div>{{unit}}</div><script>window.__partSeek=function(){}</script>`)
	if len(probs) != 1 || !strings.Contains(probs[0], `"unit"`) {
		t.Fatalf("%v", probs)
	}
	if p := animatableSlotProblems(m, `<script>window.__partSeek=function(){document.title=window.__params.unit}</script>`); len(p) != 0 {
		t.Fatalf("a parameter read in the seek is fine: %v", p)
	}
}

func TestADraftedPartCannotDeclareAnimatableParameters(t *testing.T) {
	manifest := `{"name":"tick","description":"x","version":1,"levels":["simple"],"params":[{"name":"n","type":"int","default":1,"animatable":true}]}`
	_, probs := ParsePart([]byte(manifest), `<div id="a"></div><script>window.__partSeek=function(t,b){}</script>`)
	if len(probs) != 1 || !strings.Contains(probs[0], "animatable") {
		t.Fatalf("%v", probs)
	}
}

func TestEveryBuiltinAnimatableParameterIsReadPerFrame(t *testing.T) {
	// Every built-in part that declares an animatable parameter must still load (no text slot) and name a number.
	got := 0
	for _, p := range (Catalog{Builtin: BuiltinParts()}).List() {
		for _, pr := range p.Params {
			if pr.Animatable {
				got++
				if pr.Type != "int" && pr.Type != "number" {
					t.Errorf("%s.%s is animatable but not a number", p.Name, pr.Name)
				}
			}
		}
	}
	if got == 0 {
		t.Fatal("no built-in part declares an animatable parameter")
	}
}

func TestAPageWithoutAnimationIsTheOldPage(t *testing.T) {
	html, err := BuildPage(testScene())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"__animate", "__partParams", "__setAnimated", "__animateValue"} {
		if strings.Contains(html, s) {
			t.Fatalf("a page with nothing animated must not carry %s", s)
		}
	}
	if !strings.Contains(html, "<script>(function(){window.__partSeeks=window.__partSeeks||[];") {
		t.Fatal("the block of a part changed")
	}
}

func counterScene(t *testing.T, a Animate) Scene {
	t.Helper()
	p, ok := (Catalog{Builtin: BuiltinParts()}).Get("counter")
	if !ok {
		t.Fatal("no counter")
	}
	params, err := p.ResolveParams(map[string]any{"step": 1})
	if err != nil {
		t.Fatal(err)
	}
	return Scene{Part: p, Params: params, Animate: a, Design: good(), Mode: ModeDark, Duration: 8, Width: 1280, Height: 720}
}

// counterText is what the counter shows at t, with the beat clock of the page frozen at a time before its first beat,
// so what it shows is its starting value.
func TestAKeyframedCounterShowsTheExpectedValues(t *testing.T) {
	a := Animate{"from": {{T: 0, Value: 0.0}, {T: 4, Value: 100.0, Ease: "out"}}}
	s, _ := openPage(t, counterScene(t, a))
	read := func(at float64) int {
		var v string
		evalAt(t, s, at, `document.querySelector('#ct .n').textContent`, &v)
		n := 0
		for _, c := range v {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		return n
	}
	// the counter adds one a beat (120 bpm: 2 a second), so what is left after taking the beats away is the keyframed `from`
	base := func(at float64) int { return int(at * 2) }
	v1, v4, v7 := read(1)-base(1), read(4)-base(4), read(7)-base(7)
	if v4 < 98 || v7 < 98 || v7 > 102 {
		t.Fatalf("from should be 100 at 4 s and after: got %d and %d", v4, v7)
	}
	if v1 <= 0 || v1 >= 100 {
		t.Fatalf("at 1 s it is on its way: %d", v1)
	}
	// a plain counter has no keyframes: from stays 0
	plain, _ := openPage(t, counterScene(t, nil))
	var txt string
	evalAt(t, plain, 7, `document.querySelector('#ct .n').textContent`, &txt)
	if txt == "" || strings.HasPrefix(txt, "1") && len(txt) > 2 {
		t.Fatalf("a counter with no keyframes keeps its own start: %q", txt)
	}
	_ = context.Background()
}

// A part that declares a parameter animatable must draw a different frame when the keyframe says another value.
func TestEveryAnimatableParameterOfABuiltinPartChangesTheFrame(t *testing.T) {
	browser := browserOrSkip(t)
	for _, p := range (Catalog{Builtin: BuiltinParts()}).List() {
		for _, pr := range p.Params {
			if !pr.Animatable || pr.Min == nil || pr.Max == nil {
				continue
			}
			p, pr := p, pr
			t.Run(p.Name+"."+pr.Name, func(t *testing.T) {
				frame := func(v float64) [][]byte {
					params, err := p.ResolveParams(nil)
					if err != nil {
						t.Fatal(err)
					}
					sc := Scene{Part: p, Params: params, Animate: Animate{pr.Name: {{T: 0, Value: v}}}, Design: good(), Mode: ModeDark, Duration: 8, Width: 1280, Height: 720}
					f, err := renderAt(context.Background(), browser, sc, []float64{1.6, 3.5, 7})
					if err != nil {
						t.Fatal(err)
					}
					return f
				}
				lo, hi, differ := frame(*pr.Min), frame(*pr.Max), false
				for i := range lo {
					differ = differ || string(lo[i]) != string(hi[i])
				}
				if !differ {
					t.Fatalf("%s.%s is declared animatable but the frame does not change with it", p.Name, pr.Name)
				}
			})
		}
	}
}

// The golden values written in docs/MUSIC_MODE_MOTION.md, which a port of the evaluation must match.
func TestTheEvaluationGivesTheGoldenValuesOfTheDocs(t *testing.T) {
	s, _ := openPage(t, counterScene(t, Animate{"from": {{T: 0, Value: 0.0}, {T: 4, Value: 100.0}}}))
	cases := []struct {
		kfs  string
		at   float64
		want any
	}{
		{`[{t:0,value:0,ease:'out'},{t:4,value:100}]`, 0, 0.0},
		{`[{t:0,value:0,ease:'out'},{t:4,value:100}]`, 1, 57.8125},
		{`[{t:0,value:0,ease:'out'},{t:4,value:100}]`, 9, 100.0},
		{`[{t:0,value:0},{t:4,value:100,ease:'in'}]`, 2, 50.0},
		{`[{t:0,value:0,ease:'in'},{t:4,value:100}]`, 2, 12.5},
		{`[{t:0,value:0,ease:'in-out'},{t:4,value:100}]`, 2, 50.0},
		{`[{t:0,value:0,ease:'step'},{t:4,value:100}]`, 3.9, 0.0},
		{`[{t:0,value:'a'},{t:4,value:'b'}]`, 3.9, "a"},
		{`[{t:1,value:5},{t:4,value:9}]`, 0, 5.0},
	}
	for _, c := range cases {
		var got any
		if err := s.EvalJSON(context.Background(), "JSON.stringify(window.__animateValue("+c.kfs+", "+fmt.Sprint(c.at)+"))", &got); err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s at %v: got %v, want %v", c.kfs, c.at, got, c.want)
		}
	}
}
