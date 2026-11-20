package motionref

import (
	"encoding/json"
	"strings"
	"testing"
)

func someShots() []Shot {
	yes, no := true, false
	return []Shot{
		{Index: 0, Start: 0, End: 2.1, Motion: 0.01},
		{Index: 1, Start: 2.1, End: 4, Motion: 0.3, BeatCut: &yes},
		{Index: 2, Start: 4, End: 5, Motion: 0.3, BeatCut: &yes},
		{Index: 3, Start: 5, End: 6, Motion: 0.01, BeatCut: &no},
		{Index: 4, Start: 6, End: 12.8, Motion: 0.1, BeatCut: &yes},
		{Index: 5, Start: 12.8, End: 15, Motion: 0.1},
		{Index: 6, Start: 15, End: 18, Motion: 0.5},
	}
}

func TestRhythm_NumbersAndTextWithBeatCuts(t *testing.T) {
	r := buildRhythm(someShots(), 18)
	if r.FirstCut == nil || *r.FirstCut != 2.1 {
		t.Fatalf("first cut %v", r.FirstCut)
	}
	if r.Burst == nil || r.Burst.Cuts != 4 || r.Burst.Start != 2.1 {
		t.Fatalf("burst %+v", r.Burst)
	}
	if r.Hold == nil || r.Hold.Start != 6 || r.Hold.End != 12.8 {
		t.Fatalf("hold %+v", r.Hold)
	}
	if r.LongestShot != 6.8 || r.MedianShot != 2.1 {
		t.Fatalf("longest %v median %v", r.LongestShot, r.MedianShot)
	}
	if r.BeatCutShare == nil || *r.BeatCutShare != 0.75 {
		t.Fatalf("beat share %v", r.BeatCutShare)
	}
	if r.StaticShare != 0.29 {
		t.Fatalf("static share %v", r.StaticShare)
	}
	const want = "Cuts: 30, 10, 20/min by thirds; first cut at 2.1 s; burst at 0:02–0:12 (4 cuts); longest hold 6.8 s at 0:06; median shot 2.1 s; 75 % of cuts on the beat; 29 % of shots static."
	if got := RhythmText(r); got != want {
		t.Fatalf("text:\n%s\nwant:\n%s", got, want)
	}
}

func TestRhythm_WithoutBeatCutsTheShareIsLeftOut(t *testing.T) {
	shots := someShots()
	for i := range shots {
		shots[i].BeatCut = nil
	}
	r := buildRhythm(shots, 18)
	if r.BeatCutShare != nil || strings.Contains(RhythmText(r), "beat") {
		t.Fatalf("%v %s", r.BeatCutShare, RhythmText(r))
	}
}

func TestRhythm_WithReadingsTheTextSaysHowTheTextEnters(t *testing.T) {
	shots := someShots()
	word := &ShotReading{Subject: "typography", Text: "word", TextSize: "huge", Entrance: "word", Camera: "static"}
	letter := &ShotReading{Subject: "typography", Text: "line", TextSize: "huge", Entrance: "letter", Camera: "static"}
	fig := &ShotReading{Subject: "figure", Text: "none", TextSize: "small", Entrance: "cut", Camera: "pan"}
	shots[1].Read, shots[2].Read, shots[3].Read, shots[4].Read = word, word, letter, fig
	shots[5].Read = &ShotReading{Subject: "unknown", Note: "not JSON"}
	r := buildRhythm(shots, 18)
	if r.TextLedShare == nil || *r.TextLedShare != 0.75 {
		t.Fatalf("text-led %v (three of the four read shots)", r.TextLedShare)
	}
	if r.Entrances["word"] != 2 || r.Entrances["letter"] != 1 || r.Subjects["typography"] != 3 || r.Subjects["figure"] != 1 {
		t.Fatalf("%+v %+v", r.Entrances, r.Subjects)
	}
	text := RhythmText(r)
	for _, want := range []string{"75 % of shots are text filling the frame", "entering by word (2) or letter (1)", "subjects: typography 3, figure 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q lacks %q", text, want)
		}
	}
}

func TestRhythm_TheSameSheetGivesTheSameTextUnder600Characters(t *testing.T) {
	r := buildRhythm(someShots(), 18)
	for k := 0; k < 6; k++ {
		r.Subjects = map[string]int{"typography": 38, "figure": 9, "chart": 9, "ui": 4, "photo": 3, "abstract": 3, "empty": 3}
		r.Entrances = map[string]int{"word": 31, "letter": 12, "cut": 12, "fade": 12}
		v := 0.64
		r.TextLedShare = &v
		a, b := RhythmText(r), RhythmText(r)
		if a != b || len([]rune(a)) > maxRhythmText {
			t.Fatalf("%d characters: %s", len([]rune(a)), a)
		}
	}
	if !strings.Contains(RhythmText(r), "subjects: typography 38, chart 9, figure 9, ui 4") {
		t.Fatalf("ties by name, four subjects at most: %s", RhythmText(r))
	}
}

func TestAnalysis_AnOldReferenceWithoutShotsStillLoads(t *testing.T) {
	var an Analysis
	if err := jsonUnmarshal(`{"duration":10,"fps":10,"width":320,"height":240,"frames":["00.jpg"],"colours":[],"thirds":[],"cuts":3,"mean_shot":2.5}`, &an); err != nil {
		t.Fatal(err)
	}
	if an.Shots != nil || an.Rhythm != nil || an.RhythmText != "" || an.BreakdownState != "" || an.Cuts != 3 {
		t.Fatalf("%+v", an)
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func TestRhythm_TextChangesAreCountedAndNamedAndThePhaseIsSaid(t *testing.T) {
	yes := true
	shots := someShots()
	for i := range shots {
		shots[i].BeatCut = nil
	}
	shots[2].Kind, shots[3].Kind, shots[4].Kind = KindChange, KindChange, KindCut
	shots[1].Kind, shots[5].Kind, shots[6].Kind = KindCut, KindCut, KindCut
	shots[1].BeatCut = &yes
	r := buildRhythm(shots, 18)
	r.BeatPhase = 0.5
	if r.HardCutShare != 0.67 || r.ChangesPerMinute != [3]float64{20, 0, 0} || r.CutsPerMinute != [3]float64{30, 10, 20} {
		t.Fatalf("hard %v changes %v cuts %v", r.HardCutShare, r.ChangesPerMinute, r.CutsPerMinute)
	}
	text := RhythmText(r)
	for _, want := range []string{"Cuts and text changes: 30, 10, 20/min by thirds (33 % text changes)", "100 % of cuts on the beat (half a beat off the tracked grid)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q lacks %q", text, want)
		}
	}
	// the tracked grid is not named, and a sheet with no change keeps the plain wording
	r.BeatPhase = 0
	if strings.Contains(RhythmText(r), "off the tracked grid") {
		t.Fatalf("phase 0 is not said: %s", RhythmText(r))
	}
	shots[2].Kind, shots[3].Kind = KindCut, KindCut
	if text := RhythmText(buildRhythm(shots, 18)); !strings.HasPrefix(text, "Cuts: 30, 10, 20/min by thirds;") {
		t.Fatalf("%s", text)
	}
}

func TestRhythm_TheLongestTextFitsTheBudgetWhole(t *testing.T) {
	yes := true
	shots := someShots()
	for i := range shots {
		shots[i].Kind = KindChange
	}
	shots[1].BeatCut = &yes
	r := buildRhythm(shots, 18)
	r.BeatPhase = 0.75
	r.Subjects = map[string]int{"typography": 38, "figure": 9, "chart": 9, "ui": 4, "photo": 3, "abstract": 3, "empty": 3}
	r.Entrances = map[string]int{"word": 31, "letter": 12, "cut": 12, "fade": 12}
	v := 0.64
	r.TextLedShare = &v
	text := RhythmText(r)
	if n := len([]rune(text)); n > maxRhythmText || strings.HasSuffix(text, "…") {
		t.Fatalf("%d characters, cut short: %s", n, text)
	}
}
