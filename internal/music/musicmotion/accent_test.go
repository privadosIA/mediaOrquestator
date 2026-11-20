package musicmotion

import (
	"context"
	"math"
	"strings"
	"testing"
)

// The accent of the real refusal: 2.3:1 on a light background.
func lightDesign() Design {
	return Design{Dark: "#0b0f14", Light: "#f4e4bc", Accent: "#d48806", Display: "heavy", Body: "grotesk", DefaultMode: ModeLight, Concept: "a road at dusk"}
}

func hueOf(hex string) float64 { _, _, h := hexToOKLCH(hex); return h }

func TestAdjustAccent_AnAmberOnCreamGetsDarkerWithTheSameHue(t *testing.T) {
	d := lightDesign()
	if d.Validate() == nil {
		t.Fatal("the fixture must be refused for its contrast")
	}
	fixed, note, ok := AdjustAccent(d)
	if !ok {
		t.Fatal("not adjusted")
	}
	if c := Contrast(fixed.Accent, d.Light); c < MinAccentContrast {
		t.Fatalf("%s has %.2f:1", fixed.Accent, c)
	}
	l0, _, h0 := hexToOKLCH(d.Accent)
	l1, _, h1 := hexToOKLCH(fixed.Accent)
	if l1 >= l0 {
		t.Errorf("on a light background the accent gets darker: %.2f -> %.2f", l0, l1)
	}
	if math.Abs(h1-h0) > 0.05 { // radians, about 3 degrees
		t.Errorf("the hue moved: %.3f -> %.3f", h0, h1)
	}
	if l0-l1 > accentMaxShift+0.01 {
		t.Errorf("moved %.2f, the most is %.2f", l0-l1, accentMaxShift)
	}
	for _, want := range []string{"#d48806", fixed.Accent, "3:1", "light mode"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note %q lacks %q", note, want)
		}
	}
	if err := fixed.Validate(); err != nil {
		t.Errorf("the adjusted design must be valid: %v", err)
	}
	if fixed.Dark != d.Dark || fixed.Light != d.Light || fixed.Concept != d.Concept {
		t.Error("only the accent changes")
	}
}

func TestAdjustAccent_TheOtherWayOnADarkBackground(t *testing.T) {
	d := Design{Dark: "#0b0f14", Light: "#e8f1ec", Accent: "#1b3a6b", Display: "heavy", Body: "grotesk", DefaultMode: ModeDark}
	fixed, _, ok := AdjustAccent(d)
	if !ok || Contrast(fixed.Accent, d.Dark) < MinAccentContrast {
		t.Fatalf("%v %s", ok, fixed.Accent)
	}
	if l0, _, _ := hexToOKLCH(d.Accent); func() bool { l1, _, _ := hexToOKLCH(fixed.Accent); return l1 <= l0 }() {
		t.Error("on a dark background the accent gets lighter")
	}
}

func TestAdjustAccent_NothingToDoOrNothingThatWorks(t *testing.T) {
	good := lightDesign()
	good.Accent = "#8f5a00"
	if _, note, ok := AdjustAccent(good); ok || note != "" {
		t.Error("an accent that passes is left alone")
	}
	// Both modes in use on two tones with no room for an accent between them.
	tight := Design{Dark: "#595959", Light: "#d4d4d4", Accent: "#c084fc", Display: "heavy", Body: "grotesk", DefaultMode: ModeDark, Modes: map[string]string{"chorus": ModeLight}}
	if _, _, ok := AdjustAccent(tight); ok {
		t.Error("no lightness reaches 3:1 against both tones")
	}
	if _, _, ok := AdjustAccent(Design{Accent: "green"}); ok {
		t.Error("not a colour")
	}
}

func TestAdjustAccent_AColourComesBackAsItWentIn(t *testing.T) {
	for _, hex := range []string{"#d48806", "#0b0f14", "#e8f1ec", "#2bff88", "#ff2e54"} {
		l, c, h := hexToOKLCH(hex)
		if got := okLCHToHex(l, c, h); got != hex {
			t.Errorf("%s -> %s", hex, got)
		}
	}
}

// The second refused answer of the assistant is fixed by the program instead of being asked a third time.
func TestProposeDesign_AContrastRefusedTwiceIsFixedNotAskedAThirdTime(t *testing.T) {
	d := lightDesign()
	answer := `{"dark":"` + d.Dark + `","light":"` + d.Light + `","accent":"#d48806","display":"heavy","body":"grotesk","default_mode":"light","concept":"a road at dusk"}`
	w := &systemWriter{answers: []string{answer}}
	got := ProposeDesignFull(context.Background(), w, Song{Style: "pop"})
	if got.Source != "assistant" || got.Why != "" || len(w.users) != 2 {
		t.Fatalf("%s %q after %d requests", got.Source, got.Why, len(w.users))
	}
	if got.Design.Accent == "#d48806" || Contrast(got.Design.Accent, d.Light) < MinAccentContrast || hueOf(got.Design.Accent) == 0 {
		t.Fatalf("the accent %s", got.Design.Accent)
	}
	if !strings.Contains(got.Note, "#d48806") || !strings.Contains(got.Note, got.Design.Accent) {
		t.Fatalf("note %q", got.Note)
	}
	if err := got.Design.Validate(); err != nil {
		t.Fatal(err)
	}
	// Kept with the note when the design is stored.
	dir := t.TempDir()
	st, err := Resolve(context.Background(), &systemWriter{answers: []string{answer}}, dir, Song{Style: "pop"}, true)
	if err != nil || st.Source != "assistant" || st.Note == "" {
		t.Fatalf("%v %+v", err, st)
	}
	if back, _ := LoadDesign(dir); back.Note != st.Note {
		t.Error("the note is kept with the design")
	}
	// The first refusal is still asked again (the model may fix it by itself); a second refusal for anything else is not fixed.
	w = &systemWriter{answers: []string{answer, `{"dark":"#0b0f14","light":"#f4e4bc","accent":"#d48806","display":"nope","body":"grotesk","default_mode":"light","concept":"x"}`}}
	if got := ProposeDesignFull(context.Background(), w, Song{Style: "pop"}); got.Source != "fallback" || len(w.users) != 3 || got.Note != "" {
		t.Errorf("a second refusal that is not the contrast is asked again, then falls back: %s after %d requests, note %q", got.Source, len(w.users), got.Note)
	}
}

// What a person sends is judged as before: the refusal says what it said.
func TestSetDesign_APersonsLowContrastAccentIsStillRefused(t *testing.T) {
	_, err := SetDesign(t.TempDir(), lightDesign())
	if err == nil || !strings.Contains(err.Error(), "design.accent #d48806 has a contrast of 2.3:1 with the background of the light mode (#f4e4bc); it needs 3:1") {
		t.Fatalf("%v", err)
	}
}
