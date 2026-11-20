package motion

import (
	"context"
	"strings"
	"testing"
)

func TestJudgeCut_OnlyPartlyOutsideOnTheSides(t *testing.T) {
	boxes := []TextBox{
		{Text: "inside", Left: 100, Top: 100, Right: 500, Bottom: 160},
		{Text: "cut left", Left: -80, Top: 200, Right: 300, Bottom: 260},
		{Text: "cut right", Left: 1000, Top: 300, Right: 1330, Bottom: 360},
		{Text: "not yet in", Left: 1300, Top: 400, Right: 1500, Bottom: 460},    // wholly outside: on its way in
		{Text: "credits leaving", Left: 100, Top: 700, Right: 500, Bottom: 760}, // cut at the bottom: allowed
		{Text: "rounding", Left: -0.5, Top: 20, Right: 200, Bottom: 50},
	}
	got := strings.Join(JudgeCut(boxes, 1280, 3.5), "\n")
	for _, want := range []string{`"cut left"`, `left edge`, `"cut right"`, `right edge`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, not := range []string{"inside", "not yet in", "credits leaving", "rounding"} {
		if strings.Contains(got, `"`+not+`"`) {
			t.Errorf("%q must not be reported:\n%s", not, got)
		}
	}
}

func TestJudgeOverlap_MoreThanAFifthOfTheSmallerBox(t *testing.T) {
	a := TextBox{Text: "Episodio 1", Left: 100, Top: 100, Right: 400, Bottom: 160}
	over := TextBox{Text: "Mi primer vídeo", Left: 100, Top: 140, Right: 700, Bottom: 220} // 20 of 60 px of a: 33%
	touch := TextBox{Text: "touching", Left: 100, Top: 159, Right: 400, Bottom: 200}       // 1 of 60 px: under 20%
	apart := TextBox{Text: "apart", Left: 100, Top: 300, Right: 400, Bottom: 360}
	got := JudgeOverlap([]TextBox{a, over, touch, apart}, 4)
	if len(got) < 1 || !strings.Contains(got[0], "Episodio 1") || !strings.Contains(got[0], "Mi primer vídeo") {
		t.Fatalf("the overlapping pair must be reported: %v", got)
	}
	for _, p := range got {
		if strings.Contains(p, "apart") {
			t.Errorf("a box far away is not an overlap: %s", p)
		}
	}
	if n := len(JudgeOverlap([]TextBox{a, apart}, 4)); n != 0 {
		t.Errorf("separate boxes: %d problems", n)
	}
}

const cutTitlePage = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123}
h1{position:absolute;left:-200px;top:300px;margin:0;font:700 90px sans-serif;color:#fff;white-space:nowrap;animation:fade 2s both}
@keyframes fade{from{opacity:.2}to{opacity:1}}
</style></head><body><h1>Mi primer vídeo</h1></body></html>`

const overlapPage = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123;color:#fff;font:700 90px sans-serif}
#a{position:absolute;left:100px;top:300px;margin:0}#b{position:absolute;left:100px;top:340px;margin:0}
body{animation:fade 2s both}@keyframes fade{from{opacity:.3}to{opacity:1}}
</style></head><body><div id="a">Episodio 1</div><div id="b">Mi primer vídeo</div></body></html>`

const goodCreditsPage = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123;color:#fff;font:700 60px sans-serif}
#c{position:absolute;left:300px;width:680px;text-align:center;animation:up 2s linear both}
@keyframes up{from{top:700px}to{top:-90px}}
</style></head><body><div id="c">Gracias por ver</div></body></html>`

func checkPage(t *testing.T, html string) []string {
	t.Helper()
	b := realBrowser(t)
	probs, err := Check(context.Background(), b, Page{HTML: html, Width: 1280, Height: 720, Duration: 2}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return probs
}

func TestCheck_ReportsATitleCutByTheEdge(t *testing.T) {
	got := strings.Join(checkPage(t, cutTitlePage), "\n")
	if !strings.Contains(got, "Mi primer vídeo") || !strings.Contains(got, "left edge") {
		t.Errorf("the cut title must be reported: %q", got)
	}
}

func TestCheck_ReportsOverlappingText(t *testing.T) {
	got := strings.Join(checkPage(t, overlapPage), "\n")
	if !strings.Contains(got, "overlap") {
		t.Errorf("overlapping texts must be reported: %q", got)
	}
}

func TestCheck_RollingCreditsMayLeave(t *testing.T) {
	for _, p := range checkPage(t, goodCreditsPage) {
		if strings.Contains(p, "edge") || strings.Contains(p, "overlap") {
			t.Errorf("credits that leave at the top are fine: %s", p)
		}
	}
}

func TestJudgeInvisible_TextThatIsAllHiddenAtTheEnd(t *testing.T) {
	if got := JudgeInvisible(nil, 3, 4); len(got) != 1 || !strings.Contains(got[0], "none of it is visible") {
		t.Errorf("a page with text and nothing drawn must be reported: %v", got)
	}
	if got := JudgeInvisible([]TextBox{{Text: "x", Right: 10, Bottom: 10}}, 3, 4); len(got) != 0 {
		t.Errorf("some text drawn is fine: %v", got)
	}
	if got := JudgeInvisible(nil, 0, 4); len(got) != 0 {
		t.Errorf("a page drawn without DOM text (a canvas) is not this problem: %v", got)
	}
}

// An invalid animation shorthand (a var() with extra words) leaves the text at opacity 0 for the whole clip.
const hiddenTextPage = `<!doctype html><html><head><meta charset="utf-8"><style>
:root{--ease:cubic-bezier(.22,1,.36,1)}
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123;color:#fff;font:700 90px sans-serif}
h1{margin:0;padding-top:300px;text-align:center;opacity:0;animation:in .8s var(--ease .2s) forwards}
body{animation:bg 2s linear both}@keyframes bg{from{background:#123}to{background:#234}}
@keyframes in{to{opacity:1}}
</style></head><body><h1>Mi primer vídeo</h1></body></html>`

func TestCheck_ReportsTextThatNeverAppears(t *testing.T) {
	got := strings.Join(checkPage(t, hiddenTextPage), "\n")
	if !strings.Contains(got, "none of it is visible") {
		t.Errorf("hidden text must be reported: %q", got)
	}
}
