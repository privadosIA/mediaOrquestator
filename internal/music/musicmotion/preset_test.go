package musicmotion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type presetWriter struct {
	answer string
	err    error
	got    string
}

func (f *presetWriter) Write(ctx context.Context, system, user string, max int) (string, error) {
	f.got = user
	return f.answer, f.err
}

func presetCat() Catalog { return NewCatalog(nil) }

func TestAStyleFromADullPaletteIsValid(t *testing.T) {
	src := PresetSource{Name: "Greys", MeanShot: 4.26, Colours: []PresetColour{{"#777777", 0.5}, {"#7a7a7a", 0.3}, {"#808080", 0.2}}}
	p, err := MakePreset(context.Background(), nil, presetCat(), src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(presetCat()); err != nil {
		t.Fatal(err)
	}
	if p.Rhythm != 4.3 || len(p.Parts) != 0 {
		t.Fatalf("rhythm %v parts %v", p.Rhythm, p.Parts)
	}
	if len(p.Adjustments) == 0 {
		t.Fatal("a palette that needed changes says so")
	}
}

func TestOneFlatColourStillGivesAReadableDesign(t *testing.T) {
	p, err := MakePreset(context.Background(), nil, presetCat(), PresetSource{Name: "One", Colours: []PresetColour{{"#1a2b3c", 1}}})
	if err != nil || p.Design.Validate() != nil {
		t.Fatalf("%v %+v", err, p.Design)
	}
	if Contrast(p.Design.Dark, p.Design.Light) < 4.5 {
		t.Fatal("tones that cannot be read")
	}
	if p.Rhythm != MaxPieceSeconds {
		t.Fatalf("with no shot length the usual rhythm: %v", p.Rhythm)
	}
}

func TestAnAccentWithTooLittleContrastIsAdjustedAndTheStyleSaysSo(t *testing.T) {
	// A dark blue is the most saturated colour on a dark background: it has to be lightened.
	src := PresetSource{Name: "Navy", MeanShot: 3, Colours: []PresetColour{{"#05070a", 0.6}, {"#1030a0", 0.3}, {"#e8e8e8", 0.1}}}
	p, err := MakePreset(context.Background(), nil, presetCat(), src)
	if err != nil {
		t.Fatal(err)
	}
	if c := Contrast(p.Design.Accent, p.Design.Dark); c < MinAccentContrast || p.Design.Accent == "#1030a0" {
		t.Fatalf("accent %s contrast %.1f", p.Design.Accent, c)
	}
	if saturation(p.Design.Accent) < 0.4 {
		t.Fatalf("the adjusted accent keeps its colour, it does not turn grey: %s", p.Design.Accent)
	}
	said := false
	for _, n := range p.Adjustments {
		if strings.Contains(n, "#1030a0") && strings.Contains(n, "accent") {
			said = true
		}
	}
	if !said {
		t.Fatalf("%v", p.Adjustments)
	}
	// A readable accent is left alone.
	src.Colours[1].Hex = "#22e0a0"
	p, _ = MakePreset(context.Background(), nil, presetCat(), src)
	if p.Design.Accent != "#22e0a0" {
		t.Fatalf("%s", p.Design.Accent)
	}
}

func TestALightVideoStartsInTheLightMode(t *testing.T) {
	src := PresetSource{Name: "Paper", MeanShot: 5, Colours: []PresetColour{{"#f4efe6", 0.8}, {"#d94a2b", 0.1}, {"#222222", 0.1}}}
	p, err := MakePreset(context.Background(), nil, presetCat(), src)
	if err != nil || p.Design.DefaultMode != ModeLight || p.Design.Validate() != nil {
		t.Fatalf("%v %+v", err, p.Design)
	}
}

func TestTheFontsAndTheFinishesFollowTheDescription(t *testing.T) {
	cases := []struct{ text, display, verse, hit string }{
		{"A dark terminal screen in monospace, neon green glow", "mono", VerseStyleGlow, HitStyleGlow},
		{"Chrome metallic title cards in a heavy sans", "heavy", VerseStylePlain, HitStyleMetal},
		{"Elegant serif captions on cream paper", "serif", VerseStylePlain, HitStylePlain},
		{"", "grotesk", VerseStylePlain, HitStylePlain},
	}
	for _, c := range cases {
		d, _, v, h := finishesFor(c.text)
		if d != c.display || v != c.verse || h != c.hit {
			t.Errorf("%q: %s %s %s", c.text, d, v, h)
		}
	}
}

func TestTheAssistantChoosesPartsAndAnUnknownOneIsLeftOut(t *testing.T) {
	w := &presetWriter{answer: "```json\n{\"parts\":[\"phosphor-terminal\",\"no-such-part\",\"nebula\",\"phosphor-terminal\",\"timeline-strip\"]}\n```"}
	src := PresetSource{Name: "T", MeanShot: 4, Description: "green terminal", Colours: []PresetColour{{"#0a0f0a", 1}}}
	p, err := MakePreset(context.Background(), w, presetCat(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Parts) != 2 || p.Parts[0] != "phosphor-terminal" || p.Parts[1] != "nebula" {
		t.Fatalf("unknown, repeated and manual parts are dropped: %v", p.Parts)
	}
	if !strings.Contains(w.got, "green terminal") || !strings.Contains(w.got, "- nebula:") || strings.Contains(w.got, "- timeline-strip:") {
		t.Fatalf("the assistant gets the look and the usable parts: %q", w.got)
	}
}

func TestWithoutAnAssistantOrWhenItFailsThePartsAreEmptyAndTheStyleIsMade(t *testing.T) {
	src := PresetSource{Name: "T", MeanShot: 4, Description: "green terminal", Colours: []PresetColour{{"#0a0f0a", 1}}}
	for name, w := range map[string]Writer{"none": nil, "error": &presetWriter{err: errors.New("down")}, "not json": &presetWriter{answer: "sure!"}} {
		p, err := MakePreset(context.Background(), w, presetCat(), src)
		if err != nil || len(p.Parts) != 0 || p.Design.Validate() != nil {
			t.Fatalf("%s: %v %+v", name, err, p)
		}
	}
	// No description: the assistant is not even asked.
	w := &presetWriter{answer: `{"parts":["nebula"]}`}
	src.Description = ""
	if p, _ := MakePreset(context.Background(), w, presetCat(), src); len(p.Parts) != 0 || w.got != "" {
		t.Fatal("nothing to choose from")
	}
}

func TestValidateNamesTheFieldThatIsWrong(t *testing.T) {
	good, _ := MakePreset(context.Background(), nil, presetCat(), PresetSource{Name: "G", MeanShot: 4, Colours: []PresetColour{{"#0a0f0a", 0.7}, {"#22e0a0", 0.3}}})
	cases := map[string]func(p *Preset){
		"design.accent": func(p *Preset) { p.Design.Accent = p.Design.Dark },
		"rhythm":        func(p *Preset) { p.Rhythm = 0.5 },
		"parts[0]":      func(p *Preset) { p.Parts = []string{"nope"} },
		"name":          func(p *Preset) { p.Name = " " },
	}
	for field, mutate := range cases {
		p := good
		mutate(&p)
		if err := p.Validate(presetCat()); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: %v", field, err)
		}
	}
}

func TestAnAccentThatCoversAlmostNothingLosesToOneThatShows(t *testing.T) {
	src := PresetSource{Name: "Gold", MeanShot: 5, Colours: []PresetColour{{"#090b0a", 0.76}, {"#d5d3cc", 0.14}, {"#debc33", 0.02}, {"#b02626", 0.003}}}
	p, err := MakePreset(context.Background(), nil, presetCat(), src)
	if err != nil || p.Design.Accent != "#debc33" {
		t.Fatalf("%v %s", err, p.Design.Accent)
	}
}
