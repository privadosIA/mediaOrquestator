package musicmotion

import (
	"context"
	"strings"
	"testing"
)

// --- the fonts and the colours (no browser) -----------------------------------------------------

func TestSceneStyle_WhatTheDesignAllowsPasses(t *testing.T) {
	for name, frag := range map[string]string{
		"variables":     `<style>#a{color:var(--fg);background:var(--bg);border:1vh solid var(--accent);font-family:var(--display)}</style>`,
		"a stack":       `<style>.w{font-family:var(--body), "Courier New", monospace}</style>`,
		"a mix":         `<style>#b{background:color-mix(in oklch, var(--accent) 30%, transparent)}</style>`,
		"relative":      `<style>#b{box-shadow:0 0 2vh rgb(from var(--accent) r g b / 40%)}</style>`,
		"keywords":      `<style>#b{color:currentColor;background:transparent;border-color:inherit;font-family:inherit}</style>`,
		"gradient":      `<style>#b{background:linear-gradient(to right, var(--bg), color-mix(in srgb, var(--accent) 50%, var(--bg)))}</style>`,
		"inline":        `<div style="color:var(--accent);font:900 9vh/1 var(--display)">x</div>`,
		"svg":           `<svg><circle fill="none" stroke="var(--accent)" r="4"/><path fill="url(#bad)" d="M0 0"/></svg>`,
		"an asset slot": `<div style="background:{{asset:grain}}">x</div>`,
		"selectors":     `<style>#ex .ring:not(.k){position:absolute;--size:40vh}</style>`,
	} {
		if probs := sceneStyleProblems(frag); len(probs) > 0 {
			t.Errorf("%s: %v", name, probs)
		}
	}
}

func TestSceneStyle_AnythingOfItsOwnIsRefusedAndNamed(t *testing.T) {
	cases := []struct{ name, frag, want string }{
		{"a hex", `<style>#a{color:#ff3366}</style>`, "#ff3366"},
		{"a function", `<style>#a{background:rgba(255,0,0,.4)}</style>`, "rgba()"},
		{"a name", `<style>#a{border:1px solid tomato}</style>`, "tomato"},
		{"in a mix", `<style>#a{background:color-mix(in srgb, #fff 20%, var(--bg))}</style>`, "#fff"},
		{"a literal fallback is not a variable", `<style>#a{color:red}</style>`, "red"},
		{"inline", `<div style="color:#fff">x</div>`, "#fff"},
		{"svg fill", `<svg><rect fill="#e0e0e0" width="4" height="4"/></svg>`, "#e0e0e0"},
		{"svg stroke", `<svg><path stroke="gold" d="M0 0"/></svg>`, "gold"},
		{"a variable redefined", `<style>#a{--accent:#0f0}</style>`, "#0f0"},
		{"a font", `<style>#a{font-family:"Courier New", monospace}</style>`, "Courier New"},
		{"a font shorthand", `<div style="font:900 9vh Georgia">x</div>`, "Georgia"},
		{"an svg font", `<svg><text font-family="Arial">x</text></svg>`, "Arial"},
	}
	for _, c := range cases {
		probs := sceneStyleProblems(c.frag)
		if len(probs) == 0 || !strings.Contains(strings.Join(probs, ";"), c.want) {
			t.Errorf("%s: %v, want a problem naming %q", c.name, probs, c.want)
		}
	}
	if probs := sceneStyleProblems(`<style>#a{color:#ff3366}</style>`); !strings.Contains(probs[0], "var(--accent)") {
		t.Errorf("the message must say what is allowed: %v", probs)
	}
}

// --- what the page shows (no browser: the judge reads the measures) ----------------------------

func versedScene() Scene {
	return Scene{Duration: 10, Width: 1280, Height: 720, Design: good(),
		Lines: []Line{{Text: "the app asks me twice", Start: 2, End: 5, Keyword: 2, Words: []Word{{"the", 2, 2.3, true}, {"app", 2.3, 2.7, true}, {"asks", 2.7, 3.2, true}, {"me", 3.2, 3.5, true}, {"twice", 3.5, 5, true}}}}}
}

func px(vh float64) float64 { return vh * 7.2 } // 720 px tall

func TestSceneJudge_TheFiveCases(t *testing.T) {
	good := sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app", px(14)}, {"asks me", px(6)}}}
	cases := []struct {
		name string
		m    sceneMeasure
		want string // empty: passes
	}{
		{"a good one", good, ""},
		{"a caption", sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app", px(14)}, {"VERSE // 21", px(3)}}}, `"VERSE // 21"`},
		{"a section name and a time", sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app", px(14)}, {"SEC 4.37 chorus", px(3)}}}, "scenes carry no captions"},
		{"a tiny verse", sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app", px(14)}, {"asks", px(2.5)}}}, "the words of the verse are at least 4 vh"},
		{"unreadable", sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app", px(14)}, {"me", px(1.4)}, {"twice", px(1.4)}}}, "unreadable text"},
		{"no headline", sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app asks", px(8)}}}, "the word that matters is at least 12 vh"},
		{"nothing drawn", sceneMeasure{Height: 720, Cover: 0.4}, "no text is drawn"},
		{"mostly empty", sceneMeasure{Height: 720, Cover: 0.08, Texts: []measuredText{{"app", px(14)}}}, "mostly empty"},
	}
	for _, c := range cases {
		got := strings.Join(judgeScene(versedScene(), c.m, "", 3.25, true), ";")
		switch {
		case c.want == "" && got != "":
			t.Errorf("%s: refused: %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: %q, want it to say %q", c.name, got, c.want)
		}
	}
}

func TestSceneStyle_NeutralShadowsAreAllowedOnlyInShadows(t *testing.T) {
	for name, frag := range map[string]string{
		"box-shadow black":   `<style>#a{box-shadow:0 2vh 4vh rgba(0,0,0,0.5)}</style>`,
		"box-shadow white":   `<style>#a{box-shadow:0 0 3vh rgba(255, 255, 255, .2), inset 0 0 1vh #fff}</style>`,
		"text-shadow hex":    `<style>#a{text-shadow:0 .1em .2em #000000}</style>`,
		"text-shadow name":   `<style>#a{text-shadow:0 0 1vh black}</style>`,
		"slash alpha":        `<style>#a{box-shadow:0 0 2vh rgb(0 0 0 / 40%)}</style>`,
		"shadow with a mix":  `<style>#a{box-shadow:0 0 2vh rgba(0,0,0,.4), 0 0 4vh color-mix(in srgb, var(--accent) 50%, transparent)}</style>`,
		"drop-shadow filter": `<style>#a{filter:drop-shadow(0 1vh 2vh rgba(0,0,0,.5))}</style>`,
	} {
		if probs := sceneStyleProblems(frag); len(probs) > 0 {
			t.Errorf("%s: %v", name, probs)
		}
	}
	for name, frag := range map[string]string{
		"a coloured shadow":       `<style>#a{box-shadow:0 2vh 4vh rgba(255,0,0,0.5)}</style>`,
		"black as a colour":       `<style>#a{color:rgba(0,0,0,0.5)}</style>`,
		"white as a background":   `<style>#a{background:#fff}</style>`,
		"a neutral next to a hue": `<style>#a{box-shadow:0 0 2vh rgba(0,0,0,.4), 0 0 4vh #ff3366}</style>`,
	} {
		if probs := sceneStyleProblems(frag); len(probs) == 0 {
			t.Errorf("%s: passes, want a problem", name)
		}
	}
}

func TestSceneJudge_AHeadlineTooSmallNamesTheKeywordAndTheCSS(t *testing.T) {
	m := sceneMeasure{Height: 720, Cover: 0.4, Texts: []measuredText{{"app asks me twice", px(6)}}}
	got := strings.Join(judgeScene(versedScene(), m, "", 3.25, true), ";")
	for _, want := range []string{"6.0 vh", `"asks"`, "font-size: 14vh", "transform: scale("} {
		if !strings.Contains(got, want) {
			t.Errorf("%q does not say %q", got, want)
		}
	}
}

func TestSceneJudge_WhatIsAllowedAndWhatIsNot(t *testing.T) {
	sc := versedScene()
	sc.Design.Concept = "a terminal that writes the song"
	judge := func(extra string, texts ...string) string {
		var ts []measuredText
		ts = append(ts, measuredText{"app", px(14)})
		for _, x := range texts {
			ts = append(ts, measuredText{x, px(5)})
		}
		return strings.Join(judgeScene(sc, sceneMeasure{Height: 720, Cover: 0.5, Texts: ts}, extra, 3, true), ";")
	}
	if got := judge("", "TERMINAL", "twice", "a", "me"); got != "" {
		t.Errorf("the verse, the concept and short glue words are allowed: %s", got)
	}
	if got := judge("", "ROUTE 68"); !strings.Contains(got, "ROUTE 68") {
		t.Errorf("a route number: %s", got)
	}
	if got := judge("", "21"); !strings.Contains(got, `"21"`) {
		t.Errorf("a bare number that is in no line: %s", got)
	}
	if got := judge("midnight express", "Midnight"); got != "" {
		t.Errorf("the title or the idea asked for is allowed: %s", got)
	}
	if got := judge("", "tw", "ice"); got != "" {
		t.Errorf("a word of the verse drawn in two pieces is not a caption: %s", got)
	}
	if got := judge("", "app's"); got != "" {
		t.Errorf("a word with its apostrophe: %s", got)
	}
	// An instrumental piece: no verse, so no floors for it, but any text that is not the concept is a caption.
	sc.Lines = nil
	got := strings.Join(judgeScene(sc, sceneMeasure{Height: 720, Cover: 0.5, Texts: []measuredText{{"BRIDGE", px(5)}}}, "", 5, true), ";")
	if !strings.Contains(got, "BRIDGE") || strings.Contains(got, "largest text") {
		t.Errorf("instrumental: %s", got)
	}
}

func TestSceneCheckTime_IsInTheMiddleOfTheNearestLine(t *testing.T) {
	sc := versedScene()
	if got := sceneCheckTime(sc); got != 3.5 { // the scene's middle is 5 s, the line (2-5) is the nearest
		t.Errorf("%v", got)
	}
	sc.Lines = nil
	if got := sceneCheckTime(sc); got != 5 {
		t.Errorf("no lines: %v", got)
	}
}

// --- a real browser: five scenes ----------------------------------------------------------------

const probeManifest = `{"name":"probe","description":"a test scene","version":1,"levels":["simple"],"verse":"own","params":[]}`

// probeScene draws the verse in the words div at the given size, with the given extra markup, and moves on the beat.
func probeScene(wordSize string, extra string) string {
	return `<style>#w{position:absolute;left:8vw;right:8vw;top:30vh;text-align:center;font-family:var(--display);font-size:` + wordSize + `;color:var(--fg)}
#ring{position:absolute;left:50%;top:50%;width:80vh;height:80vh;margin:-40vh 0 0 -40vh;border:1vh solid var(--accent);border-radius:50%}</style>
<div id="ring"></div><div id="w"></div>` + extra + `
<script>(function(){var L=window.__scene.lines,w=document.getElementById('w'),r=document.getElementById('ring');
window.__partSeek=function(t,beat){var s='';L.forEach(function(l){(l.words||[]).forEach(function(x){if(t>=x.s)s+=x.t+' ';});});w.textContent=s;
r.style.transform='scale('+(1+0.1*Math.max(0,beat.number%2))+')';};})();</script>`
}

func TestSceneChecks_FiveScenesInARealBrowser(t *testing.T) {
	browser := browserOrSkip(t)
	cat := Catalog{Builtin: BuiltinParts()}
	in, err := BuildSceneInput(pieceSpec(t, cat), SceneRef{Piece: "x", ScriptPiece: "p1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, fragment, want string
	}{
		{"a caption", probeScene("14vh", `<div style="position:absolute;left:3vw;top:3vh;font-size:3vh;color:var(--fg)">VERSE // 21</div>`), "scenes carry no captions"},
		{"a tiny verse", probeScene("2.6vh", ""), "the largest text"},
		{"no headline", probeScene("8vh", ""), "the word that matters is at least 12 vh"},
		{"mostly empty", strings.Replace(probeScene("14vh", ""), "width:80vh;height:80vh;margin:-40vh 0 0 -40vh", "width:10vh;height:10vh;margin:-5vh 0 0 -5vh", 1), "mostly empty"},
		{"a literal colour", probeScene("14vh", "") + `<style>#w{color:#ff3366}</style>`, "#ff3366"},
		{"a good one", probeScene("14vh", ""), ""},
		// a word set small and grown with a transform is seen big: it meets the floor; the same word not grown is told how
		{"grown with a transform", probeScene("6vh", "") + `<style>#w{left:33vw;right:auto;width:34vw;transform:scale(2.3);transform-origin:50% 0}</style>`, ""},
		{"small and not grown", probeScene("6vh", ""), "give the element that shows"},
		{"a neutral shadow", probeScene("14vh", "") + `<style>#ring{box-shadow:0 2vh 4vh rgba(0,0,0,0.5)}</style>`, ""},
		{"a coloured shadow", probeScene("14vh", "") + `<style>#ring{box-shadow:0 2vh 4vh rgba(255,0,0,0.5)}</style>`, "rgba()"},
	}
	for _, c := range cases {
		p, probs := ParsePart([]byte(probeManifest), c.fragment)
		if len(probs) > 0 {
			t.Fatalf("%s: %v", c.name, probs)
		}
		probs, err := CheckPart(context.Background(), browser, p, CheckOptions{Scene: &in.Scene, SceneRules: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := strings.Join(probs, ";")
		switch {
		case c.want == "" && got != "":
			t.Errorf("%s: refused: %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: %q, want it to say %q", c.name, got, c.want)
		}
	}
}

// The scene of the program, shown to the writer as the technique expected, follows the rules it is checked against.
func TestSceneChecks_TheExampleSceneFollowsThem(t *testing.T) {
	browser := browserOrSkip(t)
	manifest, fragment := SceneExample()
	p, probs := ParsePart([]byte(manifest), fragment)
	if len(probs) > 0 {
		t.Fatal(probs)
	}
	cat := Catalog{Builtin: BuiltinParts()}
	in, err := BuildSceneInput(pieceSpec(t, cat), SceneRef{Piece: "x", ScriptPiece: "p1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if probs, err := CheckPart(context.Background(), browser, p, CheckOptions{Scene: &in.Scene, SceneRules: true}); err != nil || len(probs) > 0 {
		t.Fatalf("%v %v", err, probs)
	}
}
