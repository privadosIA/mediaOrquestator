package musicmotion

import "testing"

func TestEveryPartThatComesWithTheProgramIsValid(t *testing.T) {
	parts := Catalog{Builtin: BuiltinParts()}.List()
	want := []string{"blank", "counter", "flat-slabs", "glass-card", "stamp", "terminal", "toggle", "sunburst", "achievement-toast", "search-box", "click-button", "roll-counter", "data-table", "dependency-tree", "password-field"}
	if len(parts) < len(want) {
		t.Fatalf("expected at least %v, got %d parts", want, len(parts))
	}
	have := map[string]Part{}
	for _, p := range parts {
		have[p.Name] = p
	}
	for _, n := range want {
		p, ok := have[n]
		if !ok {
			t.Errorf("the part %s is missing or invalid", n)
		} else if !p.Supports(LevelSimple) {
			t.Errorf("%s does not support simple", n)
		}
	}
	// What each level offers: simple has six parts, medium two more, advanced three more.
	atLevel := map[string]int{}
	for _, p := range parts {
		for _, l := range p.Levels {
			atLevel[l]++
		}
	}
	if atLevel[LevelSimple] < 14 || atLevel[LevelMedium] < 16 || atLevel[LevelAdvanced] < 19 {
		t.Errorf("parts per level: %v", atLevel)
	}
}
