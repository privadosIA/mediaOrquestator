package musicmotion

import (
	"context"
	"math"
	"strings"
	"testing"
)

const sheetText = "Cuts: 9, 14, 6/min by thirds; first cut at 2.1 s; median shot 2.1 s; 71 % of cuts on the beat."

func sheet() *ReferenceSheet {
	return &ReferenceSheet{Text: sheetText, MedianShotByThird: [3]float64{4, 2.1, 5}}
}

func TestTheDesignAndTheScriptAreToldTheSheetOfTheReference(t *testing.T) {
	song := Song{Style: "synthwave", Lyrics: "la la", SectionTags: []string{"verse"}}
	plain := designUser(song, "")
	song.Reference = sheet()
	with := designUser(song, "")
	if !strings.Contains(with, sheetText) || !strings.Contains(with, "not something to copy") {
		t.Fatalf("the design reads the paragraph as a cue:\n%s", with)
	}
	if strings.Contains(plain, "Rhythm of the reference") || strings.TrimPrefix(with, "Rhythm of the reference video the person likes (a cue for the pace and the kind of shots, not something to copy): "+sheetText+"\n") != plain {
		t.Fatalf("without a reference the message is as before:\n%s", plain)
	}
	input := in(LevelSimple)
	before := scriptUser(input, "")
	input.Reference = sheet()
	after := scriptUser(input, "")
	for _, want := range []string{sheetText, "Median shot of the reference by thirds: 4.0 s, 2.1 s, 5.0 s"} {
		if !strings.Contains(after, want) {
			t.Fatalf("the script reads %q:\n%s", want, after)
		}
	}
	if strings.Contains(before, "reference") {
		t.Fatalf("no reference, no mention:\n%s", before)
	}
}

func TestTheFixedRuleFollowsTheShotsOfTheReferenceInEachThird(t *testing.T) {
	input := in(LevelSimple)
	plain, err := FixedScript(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Reference = sheet()
	got, err := FixedScript(input)
	if err != nil {
		t.Fatal(err)
	}
	covers(t, got, 60)
	// The middle third of the 60 s song is 20 s to 40 s: its pieces follow the reference's 2.1 s (after the snap to the beat of
	// 0.5 s they are between 1.5 and 2.8), while the first third follows 4 s.
	var middle []float64
	for _, p := range got.Pieces {
		if mid := (p.Start + p.End) / 2; mid > 21 && mid < 39 {
			middle = append(middle, p.End-p.Start)
		}
	}
	if len(middle) < 5 {
		t.Fatalf("the middle third is cut in many short pieces: %v", middle)
	}
	sum := 0.0
	for _, d := range middle {
		sum += d
	}
	if mean := sum / float64(len(middle)); mean < 1.5 || mean > 2.8 {
		t.Fatalf("the middle pieces last %.2f s on average, the reference cuts every 2.1: %v", mean, middle)
	}
	if len(got.Pieces) <= len(plain.Pieces) {
		t.Fatalf("a reference that cuts faster than six seconds makes more pieces: %d against %d", len(got.Pieces), len(plain.Pieces))
	}
	// no piece is shorter than a beat or longer than eight beats, and none starts off the beat
	for _, p := range got.Pieces {
		d := p.End - p.Start
		if d < 0.5-1e-6 || d > 8*0.5+1.5+1e-6 {
			t.Fatalf("%s lasts %.2f s", p.ID, d)
		}
		if f := math.Mod(p.Start/0.5, 1); p.Start != 0 && math.Min(f, 1-f) > 0.02 && p.Start != 60 {
			t.Logf("%s starts at %.3f, off the 0.5 s grid (a section edge)", p.ID, p.Start)
		}
	}
}

func TestTheFixedRuleDoesNotSplitALineAndANoSheetReferenceChangesNothing(t *testing.T) {
	input := in(LevelSimple)
	input.Reference = sheet()
	s, err := FixedScript(input)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, p := range s.Pieces {
		for _, l := range p.Lines {
			if l.Line == nil {
				continue
			}
			if other, dup := seen[*l.Line]; dup {
				t.Fatalf("line %d is in pieces %s and %s", *l.Line, other, p.ID)
			}
			seen[*l.Line] = p.ID
		}
	}
	// A reference with no median shot (no sheet of lengths) leaves the rule as it was.
	input.Reference = &ReferenceSheet{Text: ""}
	same, err := FixedScript(input)
	plain, _ := FixedScript(in(LevelSimple))
	if err != nil || len(same.Pieces) != len(plain.Pieces) {
		t.Fatalf("%v %d against %d", err, len(same.Pieces), len(plain.Pieces))
	}
	for i := range same.Pieces {
		if same.Pieces[i].Start != plain.Pieces[i].Start || same.Pieces[i].End != plain.Pieces[i].End {
			t.Fatalf("piece %d moved", i)
		}
	}
}

func TestAProposalWithAReferenceStillCoversTheSongAndIsValid(t *testing.T) {
	input := in(LevelSimple)
	input.Reference = sheet()
	p, err := ProposeScript(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	covers(t, p.Script, 60)
	if err := p.Script.Validate(input.Cat, input.Map, input.Design); err != nil {
		t.Fatal(err)
	}
}

// The song of pieceSpec lasts 30 s and its piece p2 lies at 10 to 20 s: the middle, at 50 % of the song.
func spec2WithReference(t *testing.T, shots []ReferenceShot) ProjectSpec {
	spec := pieceSpec(t, Catalog{Builtin: BuiltinParts()})
	spec.Reference = &ReferenceShots{Duration: 100, Shots: shots}
	return spec
}

func TestTheSceneWriterGetsTheKindOfShotTheReferenceHadAtThatPlace(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	yes := true
	spec := spec2WithReference(t, []ReferenceShot{
		{Start: 0, End: 30},
		{Start: 40, End: 41.8, BeatCut: &yes, Subject: "typography", Text: "word", TextSize: "huge", Entrance: "letter", Camera: "static"}, // 40 to 41.8 s: 41 % of the video
		{Start: 48, End: 52, Subject: "chart", Text: "none", TextSize: "small", Entrance: "cut", Camera: "pan"},                            // 50 %: the nearest to the middle of the song
		{Start: 60, End: 100},
	})
	in, err := BuildSceneInput(spec, SceneRef{Piece: "x", ScriptPiece: "p2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.Archetype, "a chart") || !strings.Contains(in.Archetype, "4.0 s") || strings.Contains(in.Archetype, "single word") {
		t.Fatalf("p2 is at 50 %% of the song: %q", in.Archetype)
	}
	// p1 lies at 5 to 10 s of 30 (25 %): the nearest shot is the one at 40 to 41.8 s (41 %), read as a huge single word.
	spec = spec2WithReference(t, []ReferenceShot{
		{Start: 0, End: 4},
		{Start: 20, End: 21.8, BeatCut: &yes, Subject: "typography", Text: "word", TextSize: "huge", Entrance: "letter", Camera: "static"},
		{Start: 60, End: 100},
	})
	in, _ = BuildSceneInput(spec, SceneRef{Piece: "x", ScriptPiece: "p1"}, "")
	for _, want := range []string{"huge single word", "entering letter by letter", "static camera", "1.8 s", "cut on the beat"} {
		if !strings.Contains(in.Archetype, want) {
			t.Fatalf("the cue lacks %q: %q", want, in.Archetype)
		}
	}
	user := SceneUser(in, cat, "")
	if !strings.Contains(user, "A cue from the reference video") || !strings.Contains(user, in.Archetype) || !strings.Contains(SceneSystem(), "never as a picture to copy") {
		t.Fatalf("the writer is told it is a cue:\n%s", user)
	}
	if strings.Contains(user, "frame") && strings.Contains(user, "reference frame") {
		t.Fatal("no frame of the reference travels")
	}
}

func TestTheCueOfAShotThatWasNotReadSaysOnlyItsLength(t *testing.T) {
	no := false
	spec := spec2WithReference(t, []ReferenceShot{{Start: 0, End: 100, BeatCut: &no}})
	in, _ := BuildSceneInput(spec, SceneRef{Piece: "x", ScriptPiece: "p2"}, "")
	if in.Archetype != "In the reference video, at this moment the shot lasted 100.0 s and did not cut on the beat." {
		t.Fatalf("%q", in.Archetype)
	}
}

func TestWithoutAReferenceTheSceneInputIsAsBefore(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	in, err := BuildSceneInput(pieceSpec(t, cat), SceneRef{Piece: "x", ScriptPiece: "p2"}, "")
	if err != nil || in.Archetype != "" {
		t.Fatalf("%v %q", err, in.Archetype)
	}
	if strings.Contains(SceneUser(in, cat, ""), "reference video") {
		t.Fatal("no reference, no cue")
	}
	spec := pieceSpec(t, cat)
	spec.Reference = &ReferenceShots{Duration: 10}
	if in, _ := BuildSceneInput(spec, SceneRef{Piece: "x", ScriptPiece: "p2"}, ""); in.Archetype != "" {
		t.Fatalf("a reference with no shots says nothing: %q", in.Archetype)
	}
}

// A reference that cuts faster than a beat (a burst of cuts) does not make pieces shorter than two beats: the song is 120 bpm,
// so a beat is 0.5 s and no piece is under a second. The first version had a floor of one beat and made 90 pieces of half a
// second in a third of a song of 135 s.
func TestTheFixedRuleNeverMakesAPieceUnderTwoBeats(t *testing.T) {
	input := in(LevelSimple)
	input.Reference = &ReferenceSheet{Text: "a burst", MedianShotByThird: [3]float64{0.45, 0.5, 0.3}}
	s, err := FixedScript(input)
	if err != nil {
		t.Fatal(err)
	}
	covers(t, s, 60)
	for i, p := range s.Pieces {
		if d := p.End - p.Start; d < 2*0.5-1e-6 {
			t.Fatalf("piece %d (%s) lasts %.2f s, under two beats", i, p.ID, d)
		}
	}
	// and the same after the proposal snaps the cuts to the beat
	prop, err := ProposeScript(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range prop.Script.Pieces[:len(prop.Script.Pieces)-1] { // the last piece takes what the song leaves
		if d := p.End - p.Start; d < 2*0.5-1e-6 {
			t.Fatalf("after the snap, %s lasts %.2f s", p.ID, d)
		}
	}
}
