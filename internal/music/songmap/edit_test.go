package songmap

import (
	"errors"
	"reflect"
	"testing"
)

func editable() Map {
	return Map{Version: Version, Duration: 20, BPM: 120, Beats: []float64{0, 0.5}, Loudness: []float64{0.1}, WordsSource: SourceHeardVoice,
		Sections: []Section{{Tag: "Verse", Start: 0, End: 9.6}, {Tag: "Chorus", Start: 9.6, End: 20}},
		Words: []Word{
			{Text: "one", Start: 1, End: 1.5, Line: 0, Heard: true, Confidence: 0.9},
			{Text: "two", Start: 3, End: 3.5, Line: 1, Heard: true, Confidence: 0.9},
			{Text: "three", Start: 10, End: 10.5, Line: 2, Heard: true, Confidence: 0.8},
			{Text: "four", Start: 12, End: 12.5, Line: 3, Heard: true, Confidence: 0.8},
		}}
}

const editLyrics = "[Verse]\none\ntwo\n[Chorus]\nthree\nfour"

func f(v float64) *float64 { return &v }

func TestMoveWords_MovesKeepingTheLengthAndSaysItWasEdited(t *testing.T) {
	m := editable()
	got, err := MoveWords(m, editLyrics, []WordMove{{Index: 2, Start: 9.2}})
	if err != nil {
		t.Fatal(err)
	}
	w := got.Words[2]
	if w.Start != 9.2 || w.End != 9.7 || w.Heard || w.Confidence != 0 {
		t.Fatalf("%+v", w)
	}
	if got.WordsSource != SourceEdited || got.WordsEstimated {
		t.Fatalf("source %q", got.WordsSource)
	}
	if !got.Words[0].Heard || got.Words[0].Confidence != 0.9 {
		t.Fatalf("a word that was not named stays as it was: %+v", got.Words[0])
	}
	if got.Sections[1].Start >= 9.2 || got.Sections[1].Start < 8.7 {
		t.Fatalf("the chorus starts again where it is first sung: %+v", got.Sections)
	}
	if !reflect.DeepEqual(got.Beats, m.Beats) || got.BPM != m.BPM || !reflect.DeepEqual(got.Loudness, m.Loudness) {
		t.Fatal("beats, tempo and loudness do not change")
	}
	if m.Words[2].Start != 10 || m.WordsSource != SourceHeardVoice {
		t.Fatal("the map given is not changed")
	}
}

func TestMoveWords_AnEndIsHonoured_AndTwoWordsMoveTogether(t *testing.T) {
	got, err := MoveWords(editable(), editLyrics, []WordMove{{Index: 0, Start: 2.5, End: f(2.9)}, {Index: 1, Start: 3.2}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Words[0].Start != 2.5 || got.Words[0].End != 2.9 || got.Words[1].Start != 3.2 {
		t.Fatalf("%+v", got.Words[:2])
	}
}

func TestMoveWords_Refusals(t *testing.T) {
	cases := map[string][]WordMove{
		"no words":        nil,
		"unknown index":   {{Index: 4, Start: 5}},
		"negative index":  {{Index: -1, Start: 5}},
		"before the word": {{Index: 2, Start: 2.9}},
		"after the word":  {{Index: 1, Start: 10.1}},
		"outside":         {{Index: 3, Start: 19.9}},
		"negative time":   {{Index: 0, Start: -1}},
		"too short":       {{Index: 0, Start: 1, End: f(1.01)}},
		"twice":           {{Index: 0, Start: 1}, {Index: 0, Start: 1.1}},
	}
	for name, moves := range cases {
		m := editable()
		_, err := MoveWords(m, editLyrics, moves)
		var me *MoveError
		if !errors.As(err, &me) {
			t.Errorf("%s: err = %v, want a MoveError", name, err)
		}
		if !reflect.DeepEqual(m, editable()) {
			t.Errorf("%s: the map changed", name)
		}
	}
}

func TestMoveWords_NeighboursMovedInTheSameCallCount(t *testing.T) {
	// two moves each of which alone would cross a neighbour, but together keep the order
	if _, err := MoveWords(editable(), editLyrics, []WordMove{{Index: 1, Start: 10.2}, {Index: 2, Start: 10.4}}); err != nil {
		t.Fatalf("moving both keeps the order: %v", err)
	}
}

func TestMoveWords_WithoutLyricsTheSectionsStay(t *testing.T) {
	m := editable()
	got, err := MoveWords(m, "", []WordMove{{Index: 2, Start: 9.2}})
	if err != nil || !reflect.DeepEqual(got.Sections, m.Sections) {
		t.Fatalf("%v %+v", err, got.Sections)
	}
}

func TestMoveWords_SameBodyTwiceGivesTheSameMap(t *testing.T) {
	moves := []WordMove{{Index: 2, Start: 9.2}}
	a, _ := MoveWords(editable(), editLyrics, moves)
	b, _ := MoveWords(a, editLyrics, moves)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("not idempotent:\n%+v\n%+v", a, b)
	}
}
