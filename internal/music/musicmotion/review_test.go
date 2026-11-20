package musicmotion

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
)

// fakeVision answers with its answers in turn and records what it was asked.
type fakeVision struct {
	mu      sync.Mutex
	answers []string
	err     error
	users   []string
	images  []int
}

func (v *fakeVision) Look(ctx context.Context, system, user string, images [][]byte) (string, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.users = append(v.users, user)
	v.images = append(v.images, len(images))
	if v.err != nil {
		return "", "", v.err
	}
	i := len(v.users) - 1
	if i >= len(v.answers) {
		i = len(v.answers) - 1
	}
	return v.answers[i], "fake-vision", nil
}

// scoresJSON is the reviewer's answer with the same score (0 to 3) for every criterion; the notes go one to each criterion in turn
// and the ones left over in the loose list.
func scoresJSON(each int, notes ...string) string {
	var parts, loose []string
	for i, c := range RubricCriteria {
		note := ""
		if i < len(notes) {
			note = notes[i]
		}
		parts = append(parts, `{"id": "`+c.Key+`", "score": `+string(rune('0'+each))+`, "note": "`+note+`"}`)
	}
	for i := len(RubricCriteria); i < len(notes); i++ {
		loose = append(loose, `"`+notes[i]+`"`)
	}
	return `{"criteria": [` + strings.Join(parts, ", ") + `], "notes": [` + strings.Join(loose, ", ") + `]}`
}

func TestTheRubricIsOneTextForTheWriterAndTheReviewer(t *testing.T) {
	if len(RubricCriteria) != 6 {
		t.Fatal(len(RubricCriteria))
	}
	for _, c := range RubricCriteria {
		if !strings.Contains(SceneSystem(), c.Rule) || !strings.Contains(RubricRules(), c.Rule) {
			t.Errorf("the writer's prompt lacks the rule of %s", c.Key)
		}
		if !strings.Contains(reviewSystem(), c.Question) || !strings.Contains(reviewSystem(), `{"id": "`+c.Key+`", "score": 0 to 3`) || !strings.Contains(RubricQuestions(), c.Key+":") {
			t.Errorf("the reviewer's prompt lacks the question of %s", c.Key)
		}
	}
	// scaling and verdicts
	all := func(v float64) map[string]float64 {
		m := map[string]float64{}
		for _, c := range RubricCriteria {
			m[c.Key] = v
		}
		return m
	}
	for v, want := range map[float64]int{0: 0, 1.5: 5, 3: 10} {
		if got := ScoreOf(all(v)); got != want {
			t.Errorf("%v: %d", v, got)
		}
	}
	if ScoreOf(map[string]float64{"metaphor": 9, "lyric": -3}) != 2 { // clamped to 3 and 0; the rest missing
		t.Error("clamping")
	}
	for score, want := range map[int]string{10: VerdictPass, 7: VerdictPass, 6: VerdictWeak, 5: VerdictWeak, 4: VerdictFail, 0: VerdictFail} {
		if VerdictOf(score) != want {
			t.Errorf("%d: %s", score, VerdictOf(score))
		}
	}
}

func TestAReviewIsReadFromTheAnswerOfTheModel(t *testing.T) {
	long := strings.Repeat("x", 300)
	v := &fakeVision{answers: []string{"Sure! ```json\n" + scoresJSON(1, "a clipped word", "  two   accents ", "", long, "c", "d", "e") + "\n```"}}
	r, err := ReviewFrames(context.Background(), v, [][]byte{{1}, {2}}, []Line{{Text: "Greg used to run billing"}}, "a door", "")
	if err != nil || r.Score != 3 || r.Verdict != VerdictFail || len(r.Criteria) != len(RubricCriteria) || r.Criteria[0].ID != RubricCriteria[0].Key || r.Model != "fake-vision" || len(r.Notes) != MaxReviewNotes || r.Notes[0] != "a clipped word" || r.Notes[1] != "two accents" {
		t.Fatalf("%v %+v", err, r)
	}
	if len([]rune(r.Notes[2])) != MaxReviewNoteChars || !strings.HasSuffix(r.Notes[2], "…") {
		t.Errorf("notes are capped: %d", len([]rune(r.Notes[2])))
	}
	if v.images[0] != 1 || !strings.Contains(v.users[0], "with 2 frames") || !strings.Contains(v.users[0], "Greg used to run billing") || !strings.Contains(v.users[0], "a door") {
		t.Errorf("the reviewer is told what the frames are: %q", v.users[0])
	}
	// an unreadable answer is asked for once more
	v = &fakeVision{answers: []string{"I think it is nice", scoresJSON(3)}}
	if r, _ := ReviewFrames(context.Background(), v, [][]byte{{1}}, nil, "", ""); r.Score != 10 || r.Verdict != VerdictPass || len(v.users) != 2 {
		t.Fatalf("%+v %d", r, len(v.users))
	}
	// twice unreadable: no grade, said so
	v = &fakeVision{answers: []string{"nope", `{"scores": {}}`}}
	if r, _ := ReviewFrames(context.Background(), v, [][]byte{{1}}, nil, "", ""); r.Error == "" || r.Verdict != "" || len(v.users) != 2 {
		t.Fatalf("%+v", r)
	}
	// the vision model failing is not retried
	v = &fakeVision{err: errors.New("model not installed")}
	if r, _ := ReviewFrames(context.Background(), v, [][]byte{{1}}, nil, "", ""); !strings.Contains(r.Error, "model not installed") || len(v.users) != 1 {
		t.Fatalf("%+v", r)
	}
	if r, _ := ReviewFrames(context.Background(), nil, [][]byte{{1}}, nil, "", ""); !strings.Contains(r.Error, "no vision model") {
		t.Fatalf("%+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReviewFrames(ctx, &fakeVision{answers: []string{scoresJSON(3)}}, [][]byte{{1}}, nil, "", ""); err == nil {
		t.Fatal("a cancelled review stops")
	}
	if (ReviewAsk{Min: 7}).Rewrites() != 2 || (ReviewAsk{Min: 7, Attempts: 9}).Rewrites() != 3 || (ReviewAsk{Attempts: 2}).Rewrites() != 0 || (ReviewAsk{Min: 7, Attempts: 1}).Rewrites() != 1 {
		t.Error("rewrites")
	}
	for _, bad := range []ReviewAsk{{Min: 11}, {Min: -1}, {Attempts: 4}} {
		if bad.Validate() == nil {
			t.Errorf("%+v", bad)
		}
	}
}

// Two versions of a part that move: the second draws a different shape.
const reviewFragB = `<style>#d{position:absolute;left:560px;top:200px;width:200px;height:90px;background:var(--fg)}</style><div id="d"></div>
<script>window.__partSeek=function(t,b){document.getElementById('d').style.transform='translateX('+(b.number*53%300)+'px)';};</script>`

func TestAWeakDraftIsRewrittenFromItsReviewUntilItPasses(t *testing.T) {
	w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment), answerOf(testManifest, reviewFragB)}}
	e := draftEngine(t, w, nil)
	e.Vision = &fakeVision{answers: []string{scoresJSON(1, "two accents: red and green", "the second word is clipped"), scoresJSON(3)}}
	d, _ := e.Store.Create("a dot", "", nil)
	d.ReviewAsk = &ReviewAsk{Min: 7, Attempts: 2}
	_ = e.Store.Save(d)
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Review == nil || got.Review.Score != 10 || len(got.Review.Attempts) != 2 || got.Review.Attempts[0].Score != 3 || got.Review.Attempts[1].Score != 10 || got.Attempts != 2 {
		t.Fatalf("%+v %+v", got, got.Review)
	}
	if !strings.Contains(got.Fragment, "translateX('+(b.number*53%300)") {
		t.Fatal("the version that passed is the one left ready")
	}
	if len(w.asked) != 2 || !strings.Contains(w.asked[1], "A reviewer who looked at the previews graded it 3 out of 10") || !strings.Contains(w.asked[1], "the second word is clipped") {
		t.Fatalf("the notes are the problem the writer is told: %v", w.asked)
	}
	if _, ok := e.Cat.Get("probe"); ok {
		t.Fatal("nothing is approved by itself")
	}
}

func TestADraftThatIsNeverGoodEnoughKeepsTheBestVersion(t *testing.T) {
	w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment), answerOf(testManifest, reviewFragB), answerOf(testManifest, movingFragment)}}
	e := draftEngine(t, w, nil)
	e.Vision = &fakeVision{answers: []string{scoresJSON(0, "bad"), scoresJSON(1, "better"), scoresJSON(0, "worse")}}
	d, _ := e.Store.Create("a dot", "", nil)
	d.ReviewAsk = &ReviewAsk{Min: 9}
	_ = e.Store.Save(d)
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Review.Score != 3 || len(got.Review.Attempts) != 3 || len(w.asked) != 3 || !strings.Contains(got.Fragment, "b.number*53") {
		t.Fatalf("%+v", got.Review)
	}
	if got.Review.Attempts[0].Score != 0 || got.Review.Attempts[1].Score != 3 || got.Review.Attempts[2].Score != 0 {
		t.Fatalf("every attempt is kept: %+v", got.Review.Attempts)
	}
}

func TestAReviewThatCannotBeReadLeavesTheDraftReady(t *testing.T) {
	for name, v := range map[string]Vision{"no vision": nil, "garbage": &fakeVision{answers: []string{"not json"}}, "failing": &fakeVision{err: errors.New("down")}} {
		w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment)}}
		e := draftEngine(t, w, nil)
		e.Vision = v
		d, _ := e.Store.Create("a dot", "", nil)
		d.ReviewAsk = &ReviewAsk{Min: 7}
		_ = e.Store.Save(d)
		e.Run(context.Background(), d.ID)
		got, _ := e.Store.Get(d.ID)
		if got.State != DraftReady || got.Review == nil || got.Review.Error == "" || got.Review.Verdict != "" || len(w.asked) != 1 {
			t.Errorf("%s: %+v", name, got.Review)
		}
	}
	// a draft that did not ask for a review is what it always was
	w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment)}}
	e := draftEngine(t, w, nil)
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftReady || got.Review != nil {
		t.Fatalf("%+v", got)
	}
}

func TestTheOverallReviewNamesTheScenesThatRepeat(t *testing.T) {
	v := &fakeVision{answers: []string{`{"scores": {"consistency": 2, "variety": 0}, "notes": ["scenes 1, 4 and 6 are all a stamp"], "repeated": [[1, 4, 6], [9], [2, 99]]}`}}
	o, err := ReviewOverall(context.Background(), v, []byte{1}, []string{"p1", "p2", "p3", "p4", "p5", "p6"})
	if err != nil || o.Score != 5 || len(o.Repeated) != 1 || strings.Join(o.Repeated[0], ",") != "p1,p4,p6" || len(o.Notes) != 1 {
		t.Fatalf("%v %+v", err, o)
	}
	if !strings.Contains(v.users[0], "p1, p2, p3, p4, p5, p6") {
		t.Fatal("the reviewer is told which scene is which")
	}
	if o, _ := ReviewOverall(context.Background(), &fakeVision{answers: []string{"no"}}, []byte{1}, []string{"p1"}); o.Error == "" {
		t.Fatalf("%+v", o)
	}
}

func TestAContactSheetTilesOneFramePerScene(t *testing.T) {
	frame := func(c color.Color) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 64, 36))
		for y := 0; y < 36; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, c)
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		return b.Bytes()
	}
	sheet, err := ContactSheet([][]byte{frame(color.RGBA{255, 0, 0, 255}), frame(color.RGBA{0, 255, 0, 255}), frame(color.RGBA{0, 0, 255, 255})}, 2, 32, 18)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(sheet))
	if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 36 {
		t.Fatalf("%v %v", err, img.Bounds())
	}
	if r, _, _, _ := img.At(8, 8).RGBA(); r>>8 != 255 {
		t.Error("the first tile is the first frame")
	}
	if _, g, _, _ := img.At(40, 8).RGBA(); g>>8 != 255 {
		t.Error("the second tile is the second frame")
	}
	if _, _, b, _ := img.At(8, 28).RGBA(); b>>8 != 255 {
		t.Error("the third goes to the next row")
	}
	if _, err := ContactSheet(nil, 2, 32, 18); err == nil {
		t.Fatal("nothing to tile")
	}
}

// The four frames of a draft go to the model as ONE picture (two by two), and each criterion comes back with a score of 0 to 3 and a note.
func TestTheFramesAreOnePictureAndEveryCriterionIsScored(t *testing.T) {
	tile := func(c color.Color) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 64, 36))
		for y := 0; y < 36; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, c)
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		return b.Bytes()
	}
	frames := [][]byte{tile(color.White), tile(color.Black), tile(color.White), tile(color.Black)}
	// metaphor 1 and the other five criteria 3: 16 of 18, which is 9 of 10
	var parts []string
	for _, c := range RubricCriteria {
		sc := 3
		if c.Key == "metaphor" {
			sc = 1
		}
		parts = append(parts, `{"id":"`+c.Key+`","score":`+string(rune('0'+sc))+`,"note":"note of `+c.Key+`"}`)
	}
	v := &fakeVision{answers: []string{`{"criteria":[` + strings.Join(parts, ",") + `]}`}}
	r, err := ReviewFrames(context.Background(), v, frames, nil, "", "")
	if err != nil || r.Score != 9 || r.Verdict != VerdictPass || len(r.Criteria) != len(RubricCriteria) {
		t.Fatalf("%v %+v", err, r)
	}
	if len(v.users) != 1 || v.images[0] != 1 {
		t.Fatalf("one call with one picture: %d calls, %v images", len(v.users), v.images)
	}
	if r.Criteria[0].ID != "metaphor" || r.Criteria[0].Score != 1 || r.Criteria[0].Note != "note of metaphor" {
		t.Errorf("%+v", r.Criteria)
	}
	if len(r.Notes) == 0 || r.Notes[0] != "note of metaphor" {
		t.Errorf("the notes lead with the lowest score: %v", r.Notes)
	}
	// a criterion nobody knows is left out, and a missing one counts 0
	v = &fakeVision{answers: []string{`{"criteria":[{"id":"nonsense","score":3,"note":"x"},{"id":"people","score":3,"note":""}]}`}}
	if r, _ := ReviewFrames(context.Background(), v, frames, nil, "", ""); len(r.Criteria) != 1 || r.Score != 2 {
		t.Fatalf("%+v", r)
	}
}

// described is a reviewer's answer: the four sentences first, then every criterion scored 3 except the metaphor, which gets metaphor.
func described(metaphor int, frames ...string) string {
	var parts []string
	for _, c := range RubricCriteria {
		s := 3
		if c.Key == "metaphor" {
			s = metaphor
		}
		parts = append(parts, `{"id": "`+c.Key+`", "score": `+string(rune('0'+s))+`, "note": ""}`)
	}
	var fs []string
	for _, f := range frames {
		fs = append(fs, `"`+f+`"`)
	}
	return `{"frames": [` + strings.Join(fs, ", ") + `], "criteria": [` + strings.Join(parts, ", ") + `]}`
}

func TestTheReviewerDescribesEachFrameBeforeItScores(t *testing.T) {
	sys := reviewSystem()
	for _, want := range []string{"FIRST describe each frame in one sentence", "naming every object", "those bars are mirrors", `"frames": [`, "frame N · where in the frame · the problem → the exact change", "world and the device"} {
		if !strings.Contains(sys, want) {
			t.Errorf("the reviewer's instruction lacks %q", want)
		}
	}
	v := &fakeVision{answers: []string{described(3, "a red truck on a dusk road", "the word NIGHT in white over the road")}}
	r, _ := ReviewFrames(context.Background(), v, [][]byte{{1}}, nil, "", "a dusk road")
	if len(r.Frames) != 2 || r.Frames[0] != "a red truck on a dusk road" || r.Score != 10 {
		t.Fatalf("the sentences are kept, in order, before the scores: %+v", r)
	}
	if !strings.Contains(v.users[0], "The world of the video") || !strings.Contains(v.users[0], "a dusk road") {
		t.Errorf("the reviewer is told the world: %s", v.users[0])
	}
}

func TestAFrameANobodyCanNameCapsTheMetaphorAtOne(t *testing.T) {
	grey := described(3, "some grey bars over a dark ground", "a truck")
	r, _ := ReviewFrames(context.Background(), &fakeVision{answers: []string{grey}}, [][]byte{{1}}, nil, "", "")
	if r.Score != 9 || r.Criteria[0].ID != "metaphor" || r.Criteria[0].Score != 1 {
		t.Fatalf("metaphor 3 -> 1 on bars: 16 of 18, scaled to 9: %+v", r)
	}
	if len(r.Notes) == 0 || !strings.HasPrefix(r.Notes[0], "metaphor · a stranger sees") || !strings.Contains(r.Notes[0], "(bars)") {
		t.Errorf("the note says what the picture should have been: %v", r.Notes)
	}
	// a whole word, never a substring
	for _, ok := range []string{"a barstool in a bar", "a landscape shaped by wind", "an abstraction of nothing"} {
		if r, _ := ReviewFrames(context.Background(), &fakeVision{answers: []string{described(3, ok)}}, [][]byte{{1}}, nil, "", ""); r.Score != 10 {
			t.Errorf("%q must not cap the metaphor: %+v", ok, r)
		}
	}
	// a world that names the object is the video's own choice; an empty world never exempts
	abstractWorld := "abstract shapes drifting in a dark room"
	if r, _ := ReviewFrames(context.Background(), &fakeVision{answers: []string{described(3, "abstract shapes drifting in the dark")}}, [][]byte{{1}}, nil, "", abstractWorld); r.Score != 10 {
		t.Errorf("an abstract world exempts abstract shapes: %+v", r)
	}
	if r, _ := ReviewFrames(context.Background(), &fakeVision{answers: []string{described(3, "abstract shapes drifting in the dark")}}, [][]byte{{1}}, nil, "", ""); r.Score == 10 {
		t.Error("with no world nothing is exempt")
	}
	// a metaphor already at 1 or 0 is not raised
	if r, _ := ReviewFrames(context.Background(), &fakeVision{answers: []string{described(0, "a blob")}}, [][]byte{{1}}, nil, "", ""); r.Criteria[0].Score != 0 {
		t.Errorf("never raised: %+v", r)
	}
}

func TestThePieceReviewAsksWhetherThePicturesAloneTellTheStoryAndEachFrameCausesTheNext(t *testing.T) {
	for _, want := range []string{`"pictures_alone"`, `"causes_next"`, "with every word covered", "does each frame cause the next", "a breathing, floating or pulsing loop", "scenes 3 and 4"} {
		if !strings.Contains(overallSystem, want) {
			t.Errorf("the piece review's instruction lacks %q", want)
		}
	}
	v := &fakeVision{answers: []string{`{"scores": {"consistency": 2, "variety": 1}, "pictures_alone": "No.", "causes_next": true, "notes": ["scenes 1 and 2 · the same stamp → change the second"], "repeated": [[1, 2]]}`}}
	o, _ := ReviewOverall(context.Background(), v, []byte{1}, []string{"p1", "p2"})
	if o.PicturesAlone != "no" || o.CausesNext != "yes" || len(o.Repeated) != 1 || o.Score != 8 {
		t.Fatalf("%+v", o)
	}
	o, _ = ReviewOverall(context.Background(), &fakeVision{answers: []string{`{"scores": {"consistency": 2, "variety": 2}}`}}, []byte{1}, []string{"p1"})
	if o.PicturesAlone != "" || o.CausesNext != "" {
		t.Errorf("an answer that leaves them out leaves them empty: %+v", o)
	}
}
