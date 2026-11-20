package motion

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
)

// timelinePage declares its entrances as data: a heading that rises and fades in, three list items one
// after another, and a line that is revealed from the left.
const timelinePage = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123;color:#fff;font:700 50px sans-serif}
h1{margin:0;padding:40px}li{list-style:none;padding:8px 40px}#line{height:6px;width:600px;margin-left:40px;background:#d4a64a}
</style></head><body>
<h1 id="h">Titulo</h1><ul><li>uno</li><li>dos</li><li>tres</li></ul><div id="line"></div>
<script type="application/json" id="motion-timeline">{"version":1,"tracks":[
 {"id":"h","target":"#h","start":1,"duration":1,"ease":"linear","from":{"opacity":0,"y":40},"to":{"opacity":1,"y":0}},
 {"id":"items","target":"li","start":0.5,"duration":0.4,"stagger":0.5,"ease":"linear","from":{"opacity":0,"x":-100},"to":{"opacity":1,"x":0}},
 {"id":"line","target":"#line","start":0,"duration":2,"ease":"linear","from":{"reveal":0},"to":{"reveal":1}}
]}</script></body></html>`

func openTimeline(t *testing.T, html string, duration float64) (*Session, context.Context) {
	t.Helper()
	b := realBrowser(t)
	ctx := context.Background()
	s, err := Open(ctx, b, Page{HTML: html, Width: 1280, Height: 720, Duration: duration}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, ctx
}

func styleAt(t *testing.T, s *Session, ctx context.Context, at float64, expr string) []string {
	t.Helper()
	if _, err := s.Frame(ctx, at); err != nil {
		t.Fatal(err)
	}
	var out []string
	if err := s.EvalJSON(ctx, "JSON.stringify("+expr+")", &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func near(t *testing.T, name, got string, want, tol float64) {
	t.Helper()
	var v float64
	if _, err := fmt.Sscanf(got, "%f", &v); err != nil || math.Abs(v-want) > tol {
		t.Errorf("%s = %q, want %v", name, got, want)
	}
}

func TestTimeline_ValuesAtTheEndsAndInTheMiddle(t *testing.T) {
	s, ctx := openTimeline(t, timelinePage, 3)
	h := `[document.getElementById('h').style.opacity, document.getElementById('h').style.transform]`
	before := styleAt(t, s, ctx, 0.5, h)
	near(t, "opacity before start", before[0], 0, 0.001)
	if !strings.Contains(before[1], "translate(0px, 40px)") && !strings.Contains(before[1], "translate(0px,40px)") {
		t.Errorf("before the start the element has the from values: %q", before[1])
	}
	mid := styleAt(t, s, ctx, 1.5, h)
	near(t, "opacity in the middle", mid[0], 0.5, 0.01)
	if !strings.Contains(mid[1], "20px") {
		t.Errorf("linear halfway: y = 20px, got %q", mid[1])
	}
	after := styleAt(t, s, ctx, 2.5, h)
	near(t, "opacity after the end", after[0], 1, 0.001)
	if !strings.Contains(after[1], "0px") {
		t.Errorf("after the end the element has the to values: %q", after[1])
	}
}

func TestTimeline_StaggerStartsEachElementLater(t *testing.T) {
	s, ctx := openTimeline(t, timelinePage, 3)
	items := `Array.prototype.map.call(document.querySelectorAll('li'),function(e){return e.style.opacity})`
	got := styleAt(t, s, ctx, 0.7, items) // item 0 is halfway (0.2 of 0.4), item 1 has not started
	near(t, "item 0", got[0], 0.5, 0.02)
	near(t, "item 1", got[1], 0, 0.001)
	near(t, "item 2", got[2], 0, 0.001)
	got = styleAt(t, s, ctx, 1.7, items) // item 0 and 1 are done, item 2 is halfway
	near(t, "item 0 done", got[0], 1, 0.001)
	near(t, "item 1 done", got[1], 1, 0.001)
	near(t, "item 2 halfway", got[2], 0.5, 0.02)
}

func TestTimeline_RevealClipsFromTheLeft(t *testing.T) {
	s, ctx := openTimeline(t, timelinePage, 3)
	line := `[document.getElementById('line').style.clipPath]`
	if got := styleAt(t, s, ctx, 1, line); !strings.Contains(got[0], "50%") {
		t.Errorf("half revealed = %q, want an inset of 50%% on the right", got[0])
	}
	if got := styleAt(t, s, ctx, 2.5, line); got[0] != "none" {
		t.Errorf("fully revealed = %q, want none", got[0])
	}
}

func TestTimeline_CurvesAreEasedAndEndAtTheEnds(t *testing.T) {
	for _, ease := range []string{"linear", "in", "out", "in-out", "out-back", "[0.1,0.9,0.2,1]"} {
		e := `"` + ease + `"`
		if strings.HasPrefix(ease, "[") {
			e = ease
		}
		page := `<!doctype html><html><head><style>html,body{margin:0;width:1280px;height:720px}</style></head><body><div id="a">x</div>
<script type="application/json" id="motion-timeline">{"version":1,"tracks":[{"id":"a","target":"#a","start":0,"duration":2,"ease":` + e + `,"from":{"opacity":0},"to":{"opacity":1}}]}</script></body></html>`
		s, ctx := openTimeline(t, page, 2)
		a := `[document.getElementById('a').style.opacity]`
		near(t, ease+" at 0", styleAt(t, s, ctx, 0, a)[0], 0, 0.001)
		near(t, ease+" at the end", styleAt(t, s, ctx, 2, a)[0], 1, 0.001)
		mid := styleAt(t, s, ctx, 1, a)[0]
		var v float64
		fmt.Sscanf(mid, "%f", &v)
		switch ease {
		case "in":
			if !(v < 0.5) {
				t.Errorf("in: halfway must be below 0.5, got %v", v)
			}
		case "out":
			if !(v > 0.5) {
				t.Errorf("out: halfway must be above 0.5, got %v", v)
			}
		case "linear":
			near(t, "linear halfway", mid, 0.5, 0.01)
		}
	}
}

func TestTimeline_APageWithoutTheBlockIsLeftAlone(t *testing.T) {
	page := `<!doctype html><html><head><style>html,body{margin:0;width:1280px;height:720px}#a{opacity:.3;transform:translateX(7px)}</style></head><body><div id="a">x</div></body></html>`
	s, ctx := openTimeline(t, page, 2)
	got := styleAt(t, s, ctx, 1, `[document.getElementById('a').getAttribute('style') || '', String(window.__motionTimelineHas())]`)
	if got[0] != "" || got[1] != "false" {
		t.Errorf("nothing may be applied without the block: %v", got)
	}
}

func TestTimeline_ThePreviewMessageReplacesTheTracks(t *testing.T) {
	s, ctx := openTimeline(t, timelinePage, 3)
	if _, err := s.Frame(ctx, 1.5); err != nil {
		t.Fatal(err)
	}
	var ok []bool
	// the editor moves the heading's start to 2 s: at 1.5 s it has not started yet
	expr := `JSON.stringify([window.__motionSetTimeline({version:1,tracks:[{id:'h',target:'#h',start:2,duration:1,ease:'linear',from:{opacity:0},to:{opacity:1}}]})])`
	if err := s.EvalJSON(ctx, expr, &ok); err != nil || len(ok) != 1 || !ok[0] {
		t.Fatalf("the message handler: %v %v", ok, err)
	}
	var op []string
	if err := s.EvalJSON(ctx, `JSON.stringify([document.getElementById('h').style.opacity])`, &op); err != nil {
		t.Fatal(err)
	}
	near(t, "opacity after replacing the tracks at the same time", op[0], 0, 0.001)
}

// The preview and the renderer inject the same clock, timeline script included, so a track means the same
// in both (the preview cannot be loaded inside a render session: the clock would be injected twice).
func TestTimeline_PreviewCarriesTheSameScriptAndTheMessage(t *testing.T) {
	prev := PreviewHTML(timelinePage, 3)
	for _, want := range []string{"__motionTimelineProblems", "__motionSetTimeline", `d.type==="timeline"`, "__motionSeek"} {
		if !strings.Contains(prev, want) {
			t.Errorf("the preview must carry %q", want)
		}
	}
	if !strings.Contains(Shim(3), timelineScript) {
		t.Error("the clock the renderer injects must carry the timeline script")
	}
	if !strings.Contains(prev, strings.ReplaceAll(timelineScript, "</script", "<\\/script")) {
		t.Error("the preview must carry the same timeline script as the renderer")
	}
}

// checkTimeline runs the page check on a small page whose timeline block is the given text.
func checkTimeline(t *testing.T, block string) string {
	t.Helper()
	page := `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#123;color:#fff;font:700 50px sans-serif}
#drift{animation:slide 2s linear both}@keyframes slide{from{margin-left:0}to{margin-left:50px}}
body{animation:bg 2s linear both}@keyframes bg{from{background:#123}to{background:#234}}
</style></head><body><h1 id="h">Titulo</h1><p id="drift">musica</p>
<script type="application/json" id="motion-timeline">` + block + `</script></body></html>`
	b := realBrowser(t)
	probs, err := Check(context.Background(), b, Page{HTML: page, Width: 1280, Height: 720, Duration: 2}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(probs, "\n")
}

func TestTimelineCheck_AValidBlockPasses(t *testing.T) {
	got := checkTimeline(t, `{"version":1,"tracks":[{"id":"h","target":"#h","start":0.2,"duration":1,"ease":"out","from":{"opacity":0,"y":30},"to":{"opacity":1,"y":0}}]}`)
	if got != "" {
		t.Errorf("a valid timeline must raise no problem: %s", got)
	}
}

func TestTimelineCheck_ReportsEachKindOfProblem(t *testing.T) {
	for name, tc := range map[string]struct{ block, want string }{
		"not json":          {`{"version":1,"tracks":[`, "not valid JSON"},
		"version":           {`{"version":2,"tracks":[]}`, `"version" must be 1`},
		"no tracks":         {`{"version":1}`, `"tracks" array`},
		"no id":             {`{"version":1,"tracks":[{"target":"#h","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, `no "id"`},
		"no target":         {`{"version":1,"tracks":[{"id":"a","target":"#nothing","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, `matches no element`},
		"bad selector":      {`{"version":1,"tracks":[{"id":"a","target":"##","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, "not valid"},
		"negative start":    {`{"version":1,"tracks":[{"id":"a","target":"#h","start":-1,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, `"start"`},
		"zero duration":     {`{"version":1,"tracks":[{"id":"a","target":"#h","start":0,"duration":0,"from":{"opacity":0},"to":{"opacity":1}}]}`, `"duration"`},
		"after the end":     {`{"version":1,"tracks":[{"id":"a","target":"#h","start":1.5,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, "after the end of the clip"},
		"unknown property":  {`{"version":1,"tracks":[{"id":"a","target":"#h","start":0,"duration":1,"from":{"color":0},"to":{"color":1}}]}`, `"color" in "from" is not a property`},
		"out of range":      {`{"version":1,"tracks":[{"id":"a","target":"#h","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":3}}]}`, "out of range"},
		"unknown curve":     {`{"version":1,"tracks":[{"id":"a","target":"#h","start":0,"duration":1,"ease":"bouncy","from":{"opacity":0},"to":{"opacity":1}}]}`, `"ease" must be one of`},
		"missing from":      {`{"version":1,"tracks":[{"id":"a","target":"#h","start":0,"duration":1,"to":{"opacity":1}}]}`, `no "from"`},
		"css animation too": {`{"version":1,"tracks":[{"id":"a","target":"#drift","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}]}`, "also has a CSS animation"},
	} {
		got := checkTimeline(t, tc.block)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: want a problem with %q, got %q", name, tc.want, got)
		}
	}
}

// The example in the instructions is a working page: it passes the same check the model's pages must pass.
func TestTimelineCheck_TheExampleOfTheInstructionsIsValid(t *testing.T) {
	i := strings.Index(systemPrompt, "<!doctype html>")
	if i < 0 {
		t.Fatal("no example page in the instructions")
	}
	b := realBrowser(t)
	probs, err := Check(context.Background(), b, Page{HTML: systemPrompt[i:], Width: 1280, Height: 720, Duration: 5}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(probs) != 0 {
		t.Errorf("the example must pass the check: %v", probs)
	}
}
