package motion

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const reviseTimeline = `{"version":1,"tracks":[{"id":"title","target":"h1","start":0.2,"duration":0.9,"ease":"out","from":{"opacity":0,"y":36},"to":{"opacity":1,"y":0}}]}`

const revisePage = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;overflow:hidden;background:#101418;color:#eef0f2;font-family:Arial}
.stage{position:relative;width:1280px;height:720px;background:radial-gradient(circle at 70% 20%,#26344a,#101418 70%)}
h1{margin:0;font-size:96px;color:#fff;text-shadow:0 8px 24px rgba(0,0,0,.5)}
</style></head><body><div class="stage"><h1>Title</h1></div>
<script type="application/json" id="motion-timeline">` + reviseTimeline + `</script>
<script>window.__motionDuration=5;</script></body></html>`

const slowerTimeline = `{"version":1,"tracks":[{"id":"title","target":"h1","start":0.4,"duration":1.8,"ease":"out-back","from":{"opacity":0,"y":60},"to":{"opacity":1,"y":0}}]}`

func reviseReq() Request { return Request{Width: 1280, Height: 720, Duration: 5} }

// passes is a check that finds no problem; fails returns the problems it is given the first time it is called.
func passes(context.Context, Page) ([]string, error) { return nil, nil }

func TestExtractAndSpliceTimeline(t *testing.T) {
	got, ok := ExtractTimeline(revisePage)
	if !ok || got != reviseTimeline {
		t.Fatalf("extract: %v %q", ok, got)
	}
	spliced, ok := SpliceTimeline(revisePage, slowerTimeline)
	if !ok {
		t.Fatal("splice found no block")
	}
	// every byte outside the block is as it was: putting the old content back gives the original page
	back, _ := SpliceTimeline(spliced, reviseTimeline)
	if back != revisePage {
		t.Fatal("the splice changed something outside the timeline block")
	}
	if inner, _ := ExtractTimeline(spliced); inner != slowerTimeline {
		t.Fatalf("the new block holds %q", inner)
	}
	// the paint travels untouched
	for _, paint := range []string{"radial-gradient(circle at 70% 20%,#26344a,#101418 70%)", "text-shadow:0 8px 24px rgba(0,0,0,.5)", "window.__motionDuration=5;"} {
		if !strings.Contains(spliced, paint) {
			t.Errorf("lost %q", paint)
		}
	}
	// the attributes of the element may come in any order, with any quotes and case
	for _, tag := range []string{
		`<script id="motion-timeline" type="application/json">`,
		`<script type='application/json' id='motion-timeline'>`,
		`<SCRIPT TYPE="application/json" ID="motion-timeline">`,
	} {
		page := "<body>" + tag + reviseTimeline + "</script></body>"
		if got, ok := ExtractTimeline(page); !ok || got != reviseTimeline {
			t.Errorf("%s: %v %q", tag, ok, got)
		}
	}
	if _, ok := ExtractTimeline("<body><script>var a=1</script></body>"); ok {
		t.Error("a page without a timeline block has none to extract")
	}
	if _, ok := SpliceTimeline("<body></body>", slowerTimeline); ok {
		t.Error("a page without a timeline block cannot be spliced")
	}
}

func TestExtractTimelineJSON(t *testing.T) {
	for name, answer := range map[string]string{
		"bare":              slowerTimeline,
		"with a fence":      "```json\n" + slowerTimeline + "\n```",
		"a fence, no lang":  "```\n" + slowerTimeline + "\n```",
		"with prose":        "Sure, here it is:\n" + slowerTimeline + "\nHope it helps.",
		"the whole element": `<script type="application/json" id="motion-timeline">` + slowerTimeline + `</script>`,
		"spaces":            "\n\n  " + slowerTimeline + "  \n",
	} {
		if got := ExtractTimelineJSON(answer); got != slowerTimeline {
			t.Errorf("%s: got %q", name, got)
		}
	}
	if got := ExtractTimelineJSON("I cannot do that"); got != "I cannot do that" {
		t.Errorf("an answer without JSON is returned as it is, to be rejected by the validation: %q", got)
	}
}

func TestValidateTimeline(t *testing.T) {
	track := func(extra string) string {
		return `{"version":1,"tracks":[{"id":"a","target":".a","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}` + extra + `}]}`
	}
	good := []string{
		reviseTimeline, slowerTimeline, track(""), track(`,"ease":[0.2,1,0.3,1]`), track(`,"stagger":0.1`), track(`,"ease":"in-out"`),
		`{"version":1,"tracks":[]}`,
	}
	for _, g := range good {
		if p := ValidateTimeline(g); len(p) != 0 {
			t.Errorf("%s should be valid: %v", g, p)
		}
	}
	bad := []struct{ name, in, want string }{
		{"not JSON", `not json`, "not valid JSON"},
		{"an array", `[1]`, "must be a JSON object"},
		{"version 2", `{"version":2,"tracks":[]}`, `"version" must be 1`},
		{"no version", `{"tracks":[]}`, `"version" must be 1`},
		{"no tracks", `{"version":1}`, `needs a "tracks" array`},
		{"track not an object", `{"version":1,"tracks":[1]}`, "is not an object"},
		{"no id", strings.Replace(track(""), `"id":"a",`, "", 1), `has no "id"`},
		{"repeated id", `{"version":1,"tracks":[` + trackBody("a") + `,` + trackBody("a") + `]}`, "repeats an id"},
		{"no target", strings.Replace(track(""), `"target":".a",`, "", 1), `no "target"`},
		{"negative start", strings.Replace(track(""), `"start":0`, `"start":-1`, 1), `"start" must be`},
		{"zero duration", strings.Replace(track(""), `"duration":1`, `"duration":0`, 1), `"duration" must be`},
		{"text duration", strings.Replace(track(""), `"duration":1`, `"duration":"1"`, 1), `"duration" must be`},
		{"negative stagger", track(`,"stagger":-0.1`), `"stagger" must be`},
		{"unknown curve", track(`,"ease":"bounce"`), `"ease" must be one of`},
		{"three numbers", track(`,"ease":[0,0,1]`), `"ease" must be one of`},
		{"unknown property", strings.Replace(track(""), `"from":{"opacity":0}`, `"from":{"color":0}`, 1), `"color" in "from" is not a property`},
		{"out of range", strings.Replace(track(""), `"to":{"opacity":1}`, `"to":{"opacity":2}`, 1), "out of range (0 to 1)"},
		{"not a number", strings.Replace(track(""), `"to":{"opacity":1}`, `"to":{"opacity":"1"}`, 1), "must be a number"},
		{"no from", strings.Replace(track(""), `"from":{"opacity":0},`, "", 1), `no "from" object`},
		{"closes the script", strings.Replace(track(""), `"target":".a"`, `"target":"</script><b>"`, 1), `"</script"`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			p := ValidateTimeline(tc.in)
			if len(p) == 0 || !strings.Contains(strings.Join(p, "|"), tc.want) {
				t.Fatalf("problems %v, want one with %q", p, tc.want)
			}
		})
	}
	// the problems name the track, so the model knows which one to fix
	if p := ValidateTimeline(strings.Replace(track(""), `"duration":1`, `"duration":0`, 1)); !strings.Contains(p[0], "track 1 (a)") {
		t.Errorf("a problem must name its track: %v", p)
	}
}

func trackBody(id string) string {
	return `{"id":"` + id + `","target":".a","start":0,"duration":1,"from":{"opacity":0},"to":{"opacity":1}}`
}

// The ranges and the curves of the Go check are those of the script that runs in the page: if one changes alone the
// model would be told something the page does not accept.
func TestRangesAndCurvesMatchTheScript(t *testing.T) {
	m := regexp.MustCompile(`RANGES = \{(.*?)\};`).FindStringSubmatch(timelineScript)
	if m == nil {
		t.Fatal("RANGES not found in timelineScript")
	}
	js := map[string][2]float64{}
	for _, e := range regexp.MustCompile(`(\w+):\[([^,\]]+),([^\]]+)\]`).FindAllStringSubmatch(m[1], -1) {
		lo, err1 := strconv.ParseFloat(strings.TrimSpace(e[2]), 64)
		hi, err2 := strconv.ParseFloat(strings.TrimSpace(e[3]), 64)
		if err1 != nil || err2 != nil {
			t.Fatalf("unreadable range %v", e)
		}
		js[e[1]] = [2]float64{lo, hi}
	}
	if len(js) != len(timelineRanges) {
		t.Fatalf("the script has %d ranges, Go has %d", len(js), len(timelineRanges))
	}
	for k, v := range js {
		if timelineRanges[k] != v {
			t.Errorf("%s: script %v, Go %v", k, v, timelineRanges[k])
		}
	}
	props := append([]string(nil), TimelineProperties...)
	sort.Strings(props)
	var keys []string
	for k := range timelineRanges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(props, ",") != strings.Join(keys, ",") {
		t.Errorf("TimelineProperties %v and the ranges %v differ", props, keys)
	}
	c := regexp.MustCompile(`CURVES = \{(.*?)\};`).FindStringSubmatch(timelineScript)
	if c == nil {
		t.Fatal("CURVES not found in timelineScript")
	}
	var curves []string
	for _, e := range regexp.MustCompile(`(?:'([\w-]+)'|(\w+)):\[`).FindAllStringSubmatch(c[1], -1) {
		curves = append(curves, e[1]+e[2])
	}
	sort.Strings(curves)
	want := append([]string(nil), TimelineCurves...)
	sort.Strings(want)
	if strings.Join(curves, ",") != strings.Join(want, ",") {
		t.Errorf("curves: script %v, TimelineCurves %v", curves, want)
	}
}

func TestSystemPromptStatesWhatIsAccepted(t *testing.T) {
	s := reviseSystemPrompt()
	for _, p := range TimelineProperties {
		r := timelineRanges[p]
		if want := p + " (" + strconv.FormatFloat(r[0], 'g', -1, 64) + " to " + strconv.FormatFloat(r[1], 'g', -1, 64) + ")"; !strings.Contains(s, want) {
			t.Errorf("the instructions must state %q", want)
		}
	}
	for _, want := range append([]string{"ONE JSON object only", "never invent one", "before the clip does", "CSS animation", "motion-timeline"}, TimelineCurves...) {
		if !strings.Contains(s, want) {
			t.Errorf("the instructions must state %q", want)
		}
	}
}

func TestReviseSplicesTheNewTimelineAndKeepsThePaint(t *testing.T) {
	w := &scripted{answers: []string{"```json\n" + slowerTimeline + "\n```"}}
	var seen Page
	out, err := revise(context.Background(), w, revisePage, "make the title enter slower with a bounce", reviseReq(), nil,
		func(_ context.Context, p Page) ([]string, error) { seen = p; return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if inner, _ := ExtractTimeline(out); inner != slowerTimeline {
		t.Fatalf("the page holds %q", inner)
	}
	if back, _ := SpliceTimeline(out, reviseTimeline); back != revisePage {
		t.Fatal("the revision changed something besides the timeline")
	}
	if seen.HTML != out || seen.Width != 1280 || seen.Height != 720 || seen.Duration != 5 || seen.Transparent {
		t.Fatalf("the check must get the revised page with the clip's geometry: %+v", seen)
	}
	if len(w.users) != 1 {
		t.Fatalf("one request expected, got %d", len(w.users))
	}
	for _, want := range []string{"make the title enter slower with a bounce", "1280x720", "5.00 seconds", "PAGE", "motion-timeline", "<h1>Title</h1>"} {
		if !strings.Contains(w.users[0], want) {
			t.Errorf("the request must carry %q:\n%s", want, w.users[0])
		}
	}
}

func TestReviseAsksAgainWithWhatWasWrong(t *testing.T) {
	bad := strings.Replace(slowerTimeline, `"from":{"opacity":0,"y":60}`, `"from":{"colour":0}`, 1)
	w := &scripted{answers: []string{bad, slowerTimeline}}
	out, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), nil, passes)
	if err != nil {
		t.Fatal(err)
	}
	if inner, _ := ExtractTimeline(out); inner != slowerTimeline {
		t.Fatalf("the second answer must win: %q", inner)
	}
	if len(w.users) != 2 {
		t.Fatalf("%d requests", len(w.users))
	}
	for _, want := range []string{`"colour" in "from" is not a property`, "previous timeline", "colour", "REQUEST: slower", "<h1>Title</h1>"} {
		if !strings.Contains(w.users[1], want) {
			t.Errorf("the repair request must carry %q:\n%s", want, w.users[1])
		}
	}
}

func TestReviseSendsTheBrowserProblemsBack(t *testing.T) {
	w := &scripted{answers: []string{slowerTimeline, slowerTimeline}}
	calls := 0
	var pages []string
	out, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), nil, func(_ context.Context, p Page) ([]string, error) {
		calls++
		pages = append(pages, p.HTML)
		if calls == 1 {
			return []string{`track 1 (title): the selector "h1" matches no element of the page`}, nil
		}
		return nil, nil
	})
	if err != nil || out == "" || calls != 2 {
		t.Fatalf("err %v calls %d", err, calls)
	}
	if !strings.Contains(w.users[1], `the selector "h1" matches no element`) {
		t.Errorf("the repair request must carry the browser problem:\n%s", w.users[1])
	}
	if !strings.Contains(pages[0], slowerTimeline) {
		t.Error("the check must see the spliced page")
	}
}

func TestReviseGivesUpAfterMaxAttempts(t *testing.T) {
	w := &scripted{answers: []string{`{"version":3,"tracks":[]}`}}
	_, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), nil, passes)
	if !errors.Is(err, ErrTimelineRejected) || !strings.Contains(err.Error(), `"version" must be 1`) {
		t.Fatalf("err = %v", err)
	}
	if len(w.users) != MaxAttempts {
		t.Fatalf("%d requests, want %d", len(w.users), MaxAttempts)
	}
	// a browser that keeps refusing is the same
	w = &scripted{answers: []string{slowerTimeline}}
	_, err = revise(context.Background(), w, revisePage, "slower", reviseReq(), nil, func(context.Context, Page) ([]string, error) {
		return []string{"the page does not change over time"}, nil
	})
	if !errors.Is(err, ErrTimelineRejected) || !strings.Contains(err.Error(), "does not change over time") {
		t.Fatalf("err = %v", err)
	}
}

func TestReviseNeedsATimelineAndARequest(t *testing.T) {
	w := &scripted{answers: []string{slowerTimeline}}
	if _, err := revise(context.Background(), w, "<html><body><script>var a=1</script></body></html>", "slower", reviseReq(), nil, passes); !errors.Is(err, ErrNoTimeline) {
		t.Fatalf("err = %v", err)
	}
	if _, err := revise(context.Background(), w, revisePage, "   ", reviseReq(), nil, passes); err == nil {
		t.Fatal("an empty request must be refused")
	}
	if len(w.users) != 0 {
		t.Fatalf("the model must not be called: %d requests", len(w.users))
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write(context.Context, string, string, int) (string, error) { return "", f.err }

func TestReviseReturnsTheWritersAndTheChecksErrors(t *testing.T) {
	boom := errors.New("model is down")
	if _, err := revise(context.Background(), failingWriter{boom}, revisePage, "slower", reviseReq(), nil, passes); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	crash := errors.New("browser crashed")
	w := &scripted{answers: []string{slowerTimeline}}
	if _, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), nil, func(context.Context, Page) ([]string, error) { return nil, crash }); !errors.Is(err, crash) {
		t.Fatalf("err = %v", err)
	}
}

func TestReviseShowsTheModelTheViewWhenItKeepsTheTimeline(t *testing.T) {
	const view = `<div class="stage"><h1>Title</h1></div><script type="application/json" id="motion-timeline">` + reviseTimeline + `</script>`
	w := &scripted{answers: []string{slowerTimeline}}
	if _, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), func(context.Context, string) string { return view }, passes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.users[0], view) || strings.Contains(w.users[0], "radial-gradient") {
		t.Errorf("the model must see the view, not the paint:\n%s", w.users[0])
	}
	// a view that lost the timeline is not shown: the page is
	w = &scripted{answers: []string{slowerTimeline}}
	if _, err := revise(context.Background(), w, revisePage, "slower", reviseReq(), func(context.Context, string) string { return `<div class="stage"><h1>Title</h1></div>` }, passes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.users[0], "radial-gradient") {
		t.Errorf("a view without the timeline must be replaced by the page:\n%s", w.users[0])
	}
	// the overlay is told to the model
	w = &scripted{answers: []string{slowerTimeline}}
	r := reviseReq()
	r.Overlay = true
	var transparent bool
	if _, err := revise(context.Background(), w, revisePage, "slower", r, nil, func(_ context.Context, p Page) ([]string, error) { transparent = p.Transparent; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if !transparent || !strings.Contains(w.users[0], "OVERLAY") {
		t.Errorf("an overlay is checked as transparent and said to the model (transparent=%v)", transparent)
	}
}

// Without a browser the real check fails to open one, the way Generate's does; the request to the model was made.
func TestRevisePublicEntryPoint(t *testing.T) {
	w := &scripted{answers: []string{slowerTimeline}}
	_, err := Revise(context.Background(), w, "", revisePage, "slower", reviseReq(), Limits{}, nil)
	if len(w.users) != 1 {
		t.Fatalf("the model must be asked once: %d (err %v)", len(w.users), err)
	}
}
