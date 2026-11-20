package musicmotion

import (
	"encoding/json"
	"strings"
	"testing"
)

func style(accent string, rhythm float64, parts ...string) Preset {
	return Preset{Name: "s", Design: Design{Dark: "#101018", Light: "#f4f4f8", Accent: accent, Display: "Anton", Body: "Inter"}, Rhythm: rhythm, Parts: parts}
}

func TestStyleDistance_IdenticalIsZeroAndSymmetric(t *testing.T) {
	a := style("#22e07a", 4, "toggle", "counter")
	if d, why := StyleDistance(a, a); d != 0 || why == "" {
		t.Fatalf("identical styles: %v %q", d, why)
	}
	b := style("#ff3da8", 8, "toggle")
	d1, _ := StyleDistance(a, b)
	d2, _ := StyleDistance(b, a)
	if d1 != d2 || d1 <= 0 {
		t.Fatalf("distance is symmetric and positive: %v %v", d1, d2)
	}
}

func TestStyleDistance_ADifferentAccentAndRhythmIsFar(t *testing.T) {
	near, _ := StyleDistance(style("#22e07a", 4), style("#25e07c", 4.1))
	far, why := StyleDistance(style("#22e07a", 4, "toggle"), style("#ff3da8", 12, "counter"))
	if near >= thresholdLow {
		t.Errorf("almost the same look is below the lowest threshold: %v", near)
	}
	if far <= thresholdHigh {
		t.Errorf("a different look is above the highest threshold: %v (%s)", far, why)
	}
}

func TestStyleDistance_TheReasonNamesWhatDiffers(t *testing.T) {
	_, why := StyleDistance(style("#22e07a", 4), style("#ff3da8", 4))
	if want := "accent"; len(why) < len(want) || why[:10] != "differs in" {
		t.Fatalf("reason: %q", why)
	}
}

func TestPartsDistance_EmptyMeansAllTheUsualOnes(t *testing.T) {
	if d := partsDistance(nil, nil); d != 0 {
		t.Errorf("two empty lists: %v", d)
	}
	if d := partsDistance(nil, []string{"a"}); d != 0.5 {
		t.Errorf("empty against a list: %v", d)
	}
	if d := partsDistance([]string{"a", "b"}, []string{"a", "b"}); d != 0 {
		t.Errorf("same lists: %v", d)
	}
	if d := partsDistance([]string{"a"}, []string{"b"}); d != 1 {
		t.Errorf("disjoint lists: %v", d)
	}
}

func TestPartDistance(t *testing.T) {
	counter := PartLike{Kind: "part", Name: "counter", Description: "A big number that counts up on every beat"}
	if d, why := PartDistance(counter, counter); d != 0 || why != "the same name" {
		t.Errorf("same name: %v %q", d, why)
	}
	twin := PartLike{Name: "beat-count", Description: "A large number that counts up on each beat"}
	other := PartLike{Name: "ripple", Description: "Concentric rings spread from the centre and fade"}
	dt, _ := PartDistance(counter, twin)
	do, _ := PartDistance(counter, other)
	if dt >= do {
		t.Errorf("a twin idea is nearer than an unrelated one: %v >= %v", dt, do)
	}
	if do <= thresholdHigh {
		t.Errorf("an unrelated part is new even under high: %v", do)
	}
	if d, why := PartDistance(PartLike{Name: "x"}, other); d != 1 || why != "nothing to compare" {
		t.Errorf("nothing to compare: %v %q", d, why)
	}
}

func TestClosest_PicksTheNearestAndHandlesNone(t *testing.T) {
	if _, ok := ClosestStyle(style("#22e07a", 4), nil); ok {
		t.Error("no styles, no closest")
	}
	near := style("#23e07a", 4)
	near.ID, near.Name = "preset-near", "near"
	far := style("#ff3da8", 12)
	far.ID, far.Name = "preset-far", "far"
	got, ok := ClosestStyle(style("#22e07a", 4), []Preset{far, near})
	if !ok || got.ID != "preset-near" || got.Kind != "style" {
		t.Fatalf("closest style: %+v", got)
	}
	p, ok := ClosestPart(PartLike{Name: "ripple", Description: "rings spread"}, []PartLike{{Kind: "draft", ID: "d1", Name: "ripples", Description: "rings spread and fade"}, {Name: "counter", Description: "a number"}})
	if !ok || p.ID != "d1" || p.Kind != "draft" {
		t.Fatalf("closest part: %+v", p)
	}
}

func TestStrictness_ParseAndOrder(t *testing.T) {
	for in, want := range map[string]Strictness{"": StrictnessNormal, "Normal": StrictnessNormal, " low ": StrictnessLow, "HIGH": StrictnessHigh} {
		if got, err := ParseStrictness(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if _, err := ParseStrictness("extreme"); err == nil {
		t.Error("extreme is not a strictness")
	}
	if !(StrictnessLow.Threshold() <= StrictnessNormal.Threshold() && StrictnessNormal.Threshold() <= StrictnessHigh.Threshold()) {
		t.Fatal("low <= normal <= high")
	}
	// What is too similar under low is too similar under high, over a sweep of distances.
	for d := 0.0; d <= 1; d += 0.01 {
		s := Similar{Distance: d}
		if TooSimilar(s, StrictnessLow) && !TooSimilar(s, StrictnessHigh) {
			t.Fatalf("distance %v discarded by low but not by high", d)
		}
	}
}

func TestOrigin_IsOmittedWhenEmptyAndOldFilesStillRead(t *testing.T) {
	b, _ := json.Marshal(Preset{Name: "x"})
	if strings.Contains(string(b), "origin") {
		t.Errorf("a style made by hand carries no origin: %s", b)
	}
	b, _ = json.Marshal(Draft{ID: "d"})
	if strings.Contains(string(b), "origin") || strings.Contains(string(b), "learning_id") {
		t.Errorf("a draft asked for by hand carries no origin: %s", b)
	}
	var d Draft
	if err := json.Unmarshal([]byte(`{"id":"draft-20260101-000000-abcdef","state":"ready","request":"a"}`), &d); err != nil || d.Origin != "" || d.LearningID != "" {
		t.Errorf("a draft written before: %+v %v", d, err)
	}
	b, _ = json.Marshal(Draft{ID: "d", Origin: OriginLearned, LearningID: "learning-x"})
	if !strings.Contains(string(b), `"origin":"learned"`) || !strings.Contains(string(b), `"learning_id":"learning-x"`) {
		t.Errorf("%s", b)
	}
}
