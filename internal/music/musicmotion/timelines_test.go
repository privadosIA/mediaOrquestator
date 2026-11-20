package musicmotion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useGSAP points the page builder at the GSAP file of the repository (installed by scripts/get-gsap.sh) and skips the
// test when it is not there: the file is not committed.
func useGSAP(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		p := filepath.Join(dir, "third_party", "gsap", "gsap.min.js")
		if _, err := os.Stat(p); err == nil {
			old := GSAPFile
			GSAPFile = p
			t.Cleanup(func() { GSAPFile = old })
			return
		}
		up := filepath.Dir(dir)
		if up == dir {
			t.Skip("GSAP is not installed (bash scripts/get-gsap.sh)")
		}
		dir = up
	}
}

// timelinePart moves a box 200 px to the right over two seconds, linear, with a GSAP timeline and nothing else.
func timelinePart(id string) Part {
	return Part{
		Manifest: Manifest{Name: "tl-" + id, Description: "a box that slides with a GSAP timeline", Version: 1, Levels: []string{LevelSimple}},
		Fragment: `<div id="box-` + id + `" style="position:absolute;left:100px;top:200px;width:80px;height:80px;background:var(--accent)"></div>
<script>window.__timelines = window.__timelines || {};
var tl = gsap.timeline({paused: true}); tl.to('#box-` + id + `', {x: 200, duration: 2, ease: 'none'}, 0);
window.__timelines['main'] = tl;</script>`,
		Source: "builtin",
	}
}

func timelineScene(p Part) Scene {
	sc := testScene()
	sc.Part, sc.Params, sc.Lines, sc.Hit = p, map[string]any{}, nil, nil
	return sc
}

func TestATimelinePartIsTheSameAtTheSameTimeAndShowsTheTweenHalfWay(t *testing.T) {
	useGSAP(t)
	sc := timelineScene(timelinePart("a"))
	var frames [2][]byte
	for i := range frames {
		s, _ := openPage(t, sc)
		f, err := s.Frame(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		frames[i] = f
		var x float64
		evalAt(t, s, 1, `gsap.getProperty('#box-a', 'x')`, &x)
		if x < 99 || x > 101 {
			t.Fatalf("at 1 s the box is at x=%v, want 100 (half of the tween)", x)
		}
		evalAt(t, s, 0, `gsap.getProperty('#box-a', 'x')`, &x)
		if x != 0 {
			t.Fatalf("at 0 s x=%v, want 0 (seeking back)", x)
		}
		evalAt(t, s, 3.5, `gsap.getProperty('#box-a', 'x')`, &x)
		if x != 200 {
			t.Fatalf("after the end x=%v, want 200 (the timeline holds its last state)", x)
		}
	}
	if string(frames[0]) != string(frames[1]) {
		t.Fatal("two renders of the same timeline part give different frames at 1 s")
	}
}

func clipPart() Part {
	return Part{
		Manifest: Manifest{Name: "clips", Description: "an element that exists from 1 s for 2 s", Version: 1, Levels: []string{LevelSimple}},
		Fragment: `<div id="c" class="clip" data-start="1" data-duration="2" style="position:absolute;left:100px;top:100px;width:80px;height:80px;background:var(--accent)"></div>
<script>window.__partSeek=function(t,b){document.getElementById('c').style.opacity='0.9';};</script>`,
		Source: "builtin",
	}
}

func TestAClipIsShownOnlyOverItsRange(t *testing.T) {
	sc := timelineScene(clipPart())
	s, html := openPage(t, sc)
	if strings.Contains(html, "GreenSock") {
		t.Fatal("a part with clips and no GSAP must not carry the library")
	}
	for _, c := range []struct {
		at  float64
		vis string
	}{{0.9, "hidden"}, {1, "visible"}, {2.9, "visible"}, {3, "hidden"}} {
		var v string
		evalAt(t, s, c.at, `getComputedStyle(document.getElementById('c')).visibility`, &v)
		if v != c.vis {
			t.Fatalf("clip at %v s is %s, want %s", c.at, v, c.vis)
		}
	}
}

func bothPart() Part {
	p := timelinePart("b")
	p.Name = "both"
	p.Fragment += `<div id="dot" style="position:absolute;left:400px;top:200px;width:50px;height:50px;background:var(--accent)"></div>
<script>window.__partSeek=function(t,b){document.getElementById('dot').style.opacity=String(0.2+0.8*b.position);};</script>`
	return p
}

func TestAPartMayUseATimelineAndPartSeekTogether(t *testing.T) {
	useGSAP(t)
	s, _ := openPage(t, timelineScene(bothPart()))
	var got struct{ X, O float64 }
	evalAt(t, s, 1.25, `{X: gsap.getProperty('#box-b', 'x'), O: parseFloat(document.getElementById('dot').style.opacity)}`, &got)
	if got.X < 124 || got.X > 126 {
		t.Fatalf("the timeline did not move the box: x=%v want 125", got.X)
	}
	if got.O < 0.5 || got.O > 0.7 { // 120 bpm: a beat is 0.5 s, 1.25 s is half way into the third one
		t.Fatalf("__partSeek did not run on the beat: opacity %v want 0.6", got.O)
	}
}

func TestTheTimelineOfAnItemOfALayerRunsOnTheItemsOwnTime(t *testing.T) {
	useGSAP(t)
	sc := testScene()
	sc.Lines, sc.Hit = nil, nil
	sc.Layers = []SceneLayer{{Part: timelinePart("l"), Params: map[string]any{}, From: 1, To: 3.5}}
	sc.Extras = []ScenePart{{Part: timelinePart("e"), Params: map[string]any{}}} // the same timeline id as the layer's: they must not collide
	s, _ := openPage(t, sc)
	var got struct{ L, E float64 }
	evalAt(t, s, 2, `{L: gsap.getProperty('#box-l', 'x'), E: gsap.getProperty('#box-e', 'x')}`, &got)
	if got.L < 99 || got.L > 101 {
		t.Fatalf("the layer item is 1 s into its range, want x=100, got %v", got.L)
	}
	if got.E < 199 || got.E > 201 {
		t.Fatalf("the extra part is 2 s in, want x=200, got %v", got.E)
	}
}

func TestAPageThatNeedsGSAPWithoutItNamesTheInstallCommand(t *testing.T) {
	old := GSAPFile
	GSAPFile = filepath.Join(t.TempDir(), "none", "gsap.min.js")
	t.Cleanup(func() { GSAPFile = old })
	_, err := BuildPage(timelineScene(timelinePart("m")))
	if err == nil || !strings.Contains(err.Error(), "bash scripts/get-gsap.sh") {
		t.Fatalf("want the install command in the error, got %v", err)
	}
}

func TestAPageThatDoesNotUseTimelinesIsWhatItAlwaysWas(t *testing.T) {
	html, err := BuildPage(testScene())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"__hf", "hf-off", "gsap.", "__timelines"} {
		if strings.Contains(html, s) {
			t.Fatalf("a part without timelines must not carry %q", s)
		}
	}
}

func partWith(name, body string) Part {
	return Part{Manifest: Manifest{Name: name, Description: "a fixture of the timeline checks", Version: 1, Levels: []string{LevelSimple}}, Fragment: body, Source: "user"}
}

func checkProblems(t *testing.T, p Part) []string {
	t.Helper()
	probs, err := CheckPart(context.Background(), browserOrSkip(t), p, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return probs
}

func hasProblem(probs []string, sub string) bool {
	for _, p := range probs {
		if strings.Contains(p, sub) {
			return true
		}
	}
	return false
}

func TestATimelinePartPassesTheChecksOfAPart(t *testing.T) {
	useGSAP(t)
	if probs := checkProblems(t, timelinePart("ok")); len(probs) > 0 {
		t.Fatalf("a good timeline part is refused: %v", probs)
	}
	if !timelinePart("ok").Timeline() || pulsePart().Timeline() {
		t.Fatal("Timeline() must say which parts register a timeline")
	}
}

func TestAnEndlessRepeatIsRefusedNamingTheRule(t *testing.T) {
	p := timelinePart("r")
	p.Fragment = strings.Replace(p.Fragment, "ease: 'none'", "ease: 'none', repeat: -1", 1)
	probs, err := CheckPart(context.Background(), "", p, CheckOptions{}) // read from the text: no browser needed
	if err != nil || !hasProblem(probs, "repeat: -1") {
		t.Fatalf("want a repeat: -1 problem, got %v %v", probs, err)
	}
}

func TestATweenOfDisplayOrVisibilityOnAClipIsRefusedAndAChildIsFine(t *testing.T) {
	useGSAP(t)
	bad := partWith("clipvis", `<div id="c" class="clip" data-start="0" data-duration="4" style="position:absolute;left:100px;top:100px;width:80px;height:80px;background:var(--accent)"></div>
<script>var tl = gsap.timeline({paused: true}); var el = document.getElementById('c'); tl.to(el, {autoAlpha: 0.2, duration: 2}, 0); window.__timelines = {m: tl};</script>`)
	probs := checkProblems(t, bad)
	if !hasProblem(probs, `"autoAlpha" of the clip #c`) {
		t.Fatalf("want the clip's autoAlpha refused, got %v", probs)
	}
	good := partWith("clipchild", `<div id="c" class="clip" data-start="0" data-duration="4" style="position:absolute;left:100px;top:100px;width:80px;height:80px"><div id="k" style="width:80px;height:80px;background:var(--accent)"></div></div>
<script>var tl = gsap.timeline({paused: true}); tl.to('#k', {x: 300, duration: 2, ease: 'none'}, 0); window.__timelines = {m: tl};</script>`)
	if probs := checkProblems(t, good); len(probs) > 0 {
		t.Fatalf("animating a child of a clip is fine, got %v", probs)
	}
}

func TestACssTransformOnWhatATimelineMovesIsRefused(t *testing.T) {
	useGSAP(t)
	p := partWith("tfconflict", `<style>#b{position:absolute;left:100px;top:100px;width:80px;height:80px;background:var(--accent);transform:rotate(10deg)}</style><div id="b"></div>
<script>var tl = gsap.timeline({paused: true}); tl.to('#b', {x: 300, duration: 2, ease: 'none'}, 0); window.__timelines = {m: tl};</script>`)
	probs := checkProblems(t, p)
	if !hasProblem(probs, "CSS transform") || !hasProblem(probs, "#b") {
		t.Fatalf("want the transform conflict on #b, got %v", probs)
	}
}

func TestAClipThatEndsAfterThePartIsOnlyAWarning(t *testing.T) {
	useGSAP(t)
	p := partWith("longclip", `<div id="c" class="clip" data-start="1" data-duration="30" style="position:absolute;left:100px;top:100px;width:80px;height:80px;background:var(--accent)"></div>
<script>var tl = gsap.timeline({paused: true}); tl.to('#c', {x: 300, duration: 2, ease: 'none'}, 0); window.__timelines = {m: tl};</script>`)
	if probs := checkProblems(t, p); len(probs) > 0 {
		t.Fatalf("a clip that is too long is a warning, got problems %v", probs)
	}
}

func TestAPluginOfGSAPIsRefusedByName(t *testing.T) {
	p := partWith("plugin", `<div id="t">hello</div><script>gsap.registerPlugin(SplitText); var s = new SplitText('#t'); var tl = gsap.timeline({paused: true}); window.__timelines = {m: tl};</script>`)
	probs, err := CheckPart(context.Background(), "", p, CheckOptions{})
	if err != nil || !hasProblem(probs, "SplitText") || !hasProblem(probs, "core") {
		t.Fatalf("want the missing plugin named, got %v %v", probs, err)
	}
	// the same word in a text is not a plugin
	if probs := timelineProblems(`<div>Flip the Observer</div><script>var tl = gsap.timeline({paused: true}); window.__timelines = {m: tl};</script>`); len(probs) > 0 {
		t.Fatalf("a word in a text is not a plugin: %v", probs)
	}
}

func TestAPartWithoutGSAPOnAMachineWithoutItFailsItsCheckWithTheInstallCommand(t *testing.T) {
	old := GSAPFile
	GSAPFile = filepath.Join(t.TempDir(), "gsap.min.js")
	t.Cleanup(func() { GSAPFile = old })
	probs, err := CheckPart(context.Background(), browserOrSkip(t), timelinePart("n"), CheckOptions{})
	if err != nil || !hasProblem(probs, "bash scripts/get-gsap.sh") {
		t.Fatalf("want the install command, got %v %v", probs, err)
	}
}

func TestTheWritersAreToldTheTimelineContractAndTheRulesOfMovement(t *testing.T) {
	for name, system := range map[string]string{"scene": SceneSystem(), "part": DraftSystem()} {
		for _, want := range []string{"gsap.timeline({paused: true})", "window.__timelines", `class="clip" data-start`, "repeat: -1", "GSAP core only", "SplitText",
			"window.__partSeek(t, beat)", "entrances ease out", "nothing linear", "punch-in", "every change has a cause"} {
			if !strings.Contains(system, want) {
				t.Errorf("the %s writer must be told %q", name, want)
			}
		}
	}
	if !strings.Contains(SceneSystem(), "a scene that moves only through its timeline") || !strings.Contains(DraftSystem(), "a part that moves only through its timeline") {
		t.Error("each writer is told its own word (scene or part)")
	}
	// the rubric's criteria are the same: the movement rules are in the instruction, not in the rubric
	if len(RubricCriteria) != 6 {
		t.Errorf("the rubric has %d criteria, want the six it had", len(RubricCriteria))
	}
}

// A scene whose motion is a GSAP timeline (the lyric still drawn from __partSeek), and the same one asking for a plugin.
func timelineSceneFragment() string {
	f := strings.Replace(ownVerseFragment, `<div id="b"></div>`, `<div id="b" class="clip" data-start="0" data-duration="20"></div>`, 1)
	f = strings.Replace(f, `b.style.transform='translateX('+(Math.max(0,beat.number)*30%600)+'px)';`, ``, 1)
	return strings.Replace(f, `})();</script>`, `var tl=gsap.timeline({paused:true});tl.fromTo('#b',{x:0},{x:300,duration:8,ease:'none'},0);window.__timelines=window.__timelines||{};window.__timelines['scene']=tl;gsap.ticker.sleep();})();</script>`, 1)
}

func TestASceneWithATimelineIsDraftedCheckedAndPreviewedLikeAnyOther(t *testing.T) {
	useGSAP(t)
	plugin := strings.Replace(timelineSceneFragment(), `var tl=gsap.timeline`, `gsap.registerPlugin(SplitText);var tl=gsap.timeline`, 1)
	w := &scriptedWriter{answers: []string{answerOf(ownVerseManifest, plugin), answerOf(ownVerseManifest, timelineSceneFragment())}}
	e := draftEngine(t, w, nil)
	spec := pieceSpec(t, e.Cat)
	e.SceneInput = func(ref SceneRef, brief string) (SceneDraftInput, error) { return BuildSceneInput(spec, ref, brief) }
	d, err := e.Store.Create("(scene)", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Scene = &SceneRef{Piece: "api-mm-20261115-092531-7e5f77", ScriptPiece: "p1"}
	_ = e.Store.Save(d)
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Attempts != 2 || len(got.Preview) != DraftPreviewFrames {
		t.Fatalf("want ready at the second attempt with a preview: %+v", got)
	}
	if len(w.asked) != 2 || !strings.Contains(w.asked[1], "SplitText") || !strings.Contains(w.asked[1], "core") {
		t.Fatalf("the second request must name the plugin that is not there: %v", w.asked)
	}
	if !strings.Contains(got.Fragment, "__timelines") {
		t.Fatalf("the fragment of the draft registers a timeline: %s", got.Fragment)
	}
	distinct := map[string]bool{}
	for _, f := range got.Preview {
		distinct[string(f)] = true
	}
	if len(distinct) < 2 {
		t.Fatal("the preview frames must differ")
	}
}
