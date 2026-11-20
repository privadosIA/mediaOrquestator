package musicmotion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A style (preset) is what a piece can start from: a design, a rhythm and the parts that suit it. It is drawn from the
// analysis of a reference video (see internal/music/motionref) and always valid. See specs/motion-presets.

// Limits of the rhythm of a style, in seconds: how long a piece of the script lasts.
const (
	MinRhythm = 2.0
	MaxRhythm = 15.0
	// MaxPresetParts is the longest list of parts of a style.
	MaxPresetParts = 8
	// MaxPresetName is the longest name.
	MaxPresetName = 80
)

// Preset is a style.
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// ReferenceID is the reference it was drawn from (it may no longer exist).
	ReferenceID string    `json:"reference_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Design      Design    `json:"design"`
	// Rhythm is how long a piece lasts, in seconds.
	Rhythm float64 `json:"rhythm"`
	// Parts are catalog parts that suit it; empty means all the usual ones.
	Parts []string `json:"parts"`
	// Adjustments say what had to be changed from the colours of the video to make the design valid.
	Adjustments []string `json:"adjustments"`
	// Origin is OriginLearned for a style a learning kept; empty for one made by hand.
	Origin string `json:"origin,omitempty"`
}

// Validate checks a style: the name, the design as a whole design, the rhythm and the parts in the catalog. The error
// names the field.
func (p Preset) Validate(cat Catalog) error {
	if n := len([]rune(strings.TrimSpace(p.Name))); n == 0 || n > MaxPresetName {
		return fmt.Errorf("name must have 1 to %d characters, got %d", MaxPresetName, n)
	}
	if err := p.Design.Validate(); err != nil {
		return err
	}
	if p.Rhythm < MinRhythm || p.Rhythm > MaxRhythm || math.IsNaN(p.Rhythm) {
		return fmt.Errorf("rhythm must be between %.0f and %.0f seconds, got %v", MinRhythm, MaxRhythm, p.Rhythm)
	}
	if len(p.Parts) > MaxPresetParts {
		return fmt.Errorf("parts has %d entries, the most is %d", len(p.Parts), MaxPresetParts)
	}
	seen := map[string]bool{}
	for i, n := range p.Parts {
		if _, ok := cat.Get(n); !ok {
			return fmt.Errorf("parts[%d] %q is not a part of the catalog", i, n)
		}
		if seen[n] {
			return fmt.Errorf("parts[%d] %q is repeated", i, n)
		}
		seen[n] = true
	}
	return nil
}

// PresetColour is a dominant colour of a video with the share of it.
type PresetColour struct {
	Hex   string
	Share float64
}

// PresetSource is what a style is drawn from.
type PresetSource struct {
	Name        string
	ReferenceID string
	Colours     []PresetColour
	// MeanShot is the mean length of a shot, in seconds.
	MeanShot float64
	// Description is the look of the video as the vision model wrote it; empty when there is none.
	Description string
}

// MakePreset draws a style from a source. The design comes from the colours by rules (so it is valid with no model),
// the fonts and finishes from what the description says, the rhythm from the length of a shot, and the parts from the
// assistant (w) checked against the catalog; with no assistant, or when it fails, the list is empty.
func MakePreset(ctx context.Context, w Writer, cat Catalog, src PresetSource) (Preset, error) {
	d, notes := designFromColours(src)
	display, body, verse, hit := finishesFor(src.Description)
	d.Display, d.Body, d.VerseStyle, d.HitStyle = display, body, verse, hit
	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = "Style"
	}
	if r := []rune(name); len(r) > MaxPresetName {
		name = string(r[:MaxPresetName])
	}
	d.Concept = truncate("the look of "+name, MaxConcept)
	rhythm := src.MeanShot
	if rhythm <= 0 {
		rhythm = MaxPieceSeconds
	}
	rhythm = math.Round(math.Min(MaxRhythm, math.Max(MinRhythm, rhythm))*10) / 10
	p := Preset{Name: name, ReferenceID: src.ReferenceID, CreatedAt: time.Now().UTC(), Design: d, Rhythm: rhythm, Adjustments: notes,
		Parts: ChoosePresetParts(ctx, w, src.Description, cat)}
	if err := p.Validate(cat); err != nil {
		return Preset{}, fmt.Errorf("the style drawn is not valid: %w", err)
	}
	return p, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// --- colours -------------------------------------------------------------------------------------------------

func parseHex(hex string) (r, g, b float64, ok bool) {
	if !hexColour.MatchString(hex) {
		return 0, 0, 0, false
	}
	h := hex[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var c [3]float64
	for i := 0; i < 3; i++ {
		v, _ := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		c[i] = float64(v)
	}
	return c[0], c[1], c[2], true
}

func toHex(r, g, b float64) string {
	c := func(v float64) int { return int(math.Round(math.Max(0, math.Min(255, v)))) }
	return fmt.Sprintf("#%02x%02x%02x", c(r), c(g), c(b))
}

// mixHex moves a colour a share t of the way towards another.
func mixHex(a, b string, t float64) string {
	ar, ag, ab, _ := parseHex(a)
	br, bg, bb, _ := parseHex(b)
	return toHex(ar+(br-ar)*t, ag+(bg-ag)*t, ab+(bb-ab)*t)
}

// shiftLightness changes the HSL lightness of a colour by delta (of 0 to 1), keeping its hue and saturation.
func shiftLightness(hex string, delta float64) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return hex
	}
	r, g, b = r/255, g/255, b/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (hi + lo) / 2
	var h, s float64
	if hi != lo {
		d := hi - lo
		if l > 0.5 {
			s = d / (2 - hi - lo)
		} else {
			s = d / (hi + lo)
		}
		switch hi {
		case r:
			h = (g - b) / d
			if g < b {
				h += 6
			}
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h /= 6
	}
	l = math.Max(0, math.Min(1, l+delta))
	if s == 0 {
		return toHex(l*255, l*255, l*255)
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	pp := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return pp + (q-pp)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return pp + (q-pp)*(2.0/3-t)*6
		}
		return pp
	}
	return toHex(hue(h+1.0/3)*255, hue(h)*255, hue(h-1.0/3)*255)
}

// saturation is the HSV saturation of a colour, 0 to 1.
func saturation(hex string) float64 {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return 0
	}
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	if hi == 0 {
		return 0
	}
	return (hi - lo) / hi
}

const (
	// maxDarkLuminance is the most luminance a colour may have to be the dark tone, a background.
	maxDarkLuminance    = 0.10
	minAccentSaturation = 0.25
	neutralLight        = "#f2f2f2"
	neutralDarkAccent   = "#d9d9d9"
	neutralLightAccent  = "#262626"
)

// designFromColours makes the tones, the accent and the default mode from the dominant colours, and lists what it had
// to adjust.
func designFromColours(src PresetSource) (Design, []string) {
	var cols []PresetColour
	for _, c := range src.Colours {
		if _, _, _, ok := parseHex(c.Hex); ok {
			cols = append(cols, c)
		}
	}
	notes := []string{}
	if len(cols) == 0 {
		cols = []PresetColour{{Hex: "#101418", Share: 1}}
		notes = append(notes, "the video gave no usable colour: a neutral dark palette is used")
	}
	byLum := append([]PresetColour(nil), cols...)
	sort.SliceStable(byLum, func(i, j int) bool {
		a, _ := luminance(byLum[i].Hex)
		b, _ := luminance(byLum[j].Hex)
		return a < b
	})
	dark, light := byLum[0].Hex, byLum[len(byLum)-1].Hex
	if l, _ := luminance(dark); l > maxDarkLuminance {
		was := dark
		for i := 0; i < 40; i++ {
			if l, _ := luminance(dark); l <= maxDarkLuminance {
				break
			}
			dark = mixHex(dark, "#000000", 0.15)
		}
		notes = append(notes, fmt.Sprintf("the darkest colour %s is too light for a background: the dark tone is %s", was, dark))
	}
	if len(byLum) == 1 || Contrast(dark, light) < 4.5 {
		was := light
		if len(byLum) == 1 {
			light = neutralLight
		}
		for i := 0; i < 40 && Contrast(dark, light) < 4.5; i++ {
			light = mixHex(light, "#ffffff", 0.12)
		}
		if was != light {
			notes = append(notes, fmt.Sprintf("the light tone had to be %s (the video had %s) to be readable on %s", light, was, dark))
		}
	}
	// A light video starts in the light mode.
	mean := 0.0
	for _, c := range cols {
		l, _ := luminance(c.Hex)
		mean += l * c.Share
	}
	mode := ModeDark
	if mean > 0.5 {
		mode = ModeLight
	}
	d := Design{Dark: dark, Light: light, DefaultMode: mode}
	bg, _, _ := d.Colours(mode)

	// The most saturated colour is the accent, lightened or darkened until it contrasts with the background.
	best, bestSat := "", 0.0
	score := 0.0
	for _, c := range cols {
		sat := saturation(c.Hex)
		// A colour that covers almost nothing is a speck, not an accent: its saturation counts less the less it shows
		// (full from 2 % of the picture).
		if s := sat * math.Min(1, math.Sqrt(c.Share/0.02)); s > score && sat >= minAccentSaturation {
			best, bestSat, score = c.Hex, sat, s
		}
	}
	switch {
	case best == "" || bestSat < minAccentSaturation:
		d.Accent = neutralDarkAccent
		if mode == ModeLight {
			d.Accent = neutralLightAccent
		}
		notes = append(notes, "no colour of the video has enough colour to be an accent: a neutral accent is used")
	default:
		// Lightened (on a dark background) or darkened (on a light one) in small steps that keep the hue and the
		// saturation, so a dark purple becomes a bright purple and not a grey.
		acc, step := best, 0.04
		if mode == ModeLight {
			step = -step
		}
		for i := 0; i < 40 && Contrast(acc, bg) < MinAccentContrast; i++ {
			acc = shiftLightness(acc, step)
		}
		if acc != best {
			notes = append(notes, fmt.Sprintf("the accent %s had too little contrast with the background %s: it is %s", best, bg, acc))
		}
		d.Accent = acc
	}
	return d, notes
}

// --- fonts and finishes --------------------------------------------------------------------------------------

// finishRule maps words of a description to a choice; the first rule of each kind that matches wins.
type finishRule struct {
	words []string
	value string
}

var (
	fontRules = []finishRule{
		{[]string{"monospace", "monospaced", "mono ", "terminal", "code", "typewriter"}, "mono"},
		{[]string{"serif"}, "serif"},
		{[]string{"rounded"}, "rounded"},
		{[]string{"condensed", "narrow"}, "condensed"},
		{[]string{"heavy", "black weight", "impact", "bold"}, "heavy"},
	}
	verseRules = []finishRule{
		{[]string{"neon", "glow", "glowing", "luminous"}, VerseStyleGlow},
		{[]string{"karaoke"}, VerseStyleKaraoke},
		{[]string{"highlight", "marker"}, VerseStyleHighlight},
	}
	hitRules = []finishRule{
		{[]string{"metal", "metallic", "chrome", "steel"}, HitStyleMetal},
		{[]string{"neon", "glow", "glowing", "luminous"}, HitStyleGlow},
	}
)

func pick(text string, rules []finishRule, def string) string {
	for _, r := range rules {
		for _, w := range r.words {
			if strings.Contains(text, w) {
				return r.value
			}
		}
	}
	return def
}

// finishesFor chooses the two fonts and the two finishes from the words of a description (plain defaults without one).
func finishesFor(description string) (display, body, verse, hit string) {
	text := strings.ToLower(description) + " "
	font := pick(text, fontRules, "")
	display, body = "grotesk", "grotesk"
	if font != "" {
		display = font
		body = font
		if font == "heavy" || font == "condensed" {
			body = "grotesk" // a heavy face is for the big words, not for a verse
		}
	}
	return display, body, pick(text, verseRules, VerseStylePlain), pick(text, hitRules, HitStylePlain)
}

// --- the parts the assistant chooses ---------------------------------------------------------------------------

const presetPartsSystem = `You choose which animated parts of a catalog suit the look of a reference video, for a music motion piece.
Answer with ONE JSON object and nothing else: {"parts":["name", ...]} with between 2 and %d names from the catalog, the ones that best suit the look.`

// ChoosePresetParts asks the assistant for the parts of the catalog that suit a look and keeps the ones the catalog
// has (no repeats, at most MaxPresetParts). With no assistant, no description or an answer that holds no part, it is
// empty.
func ChoosePresetParts(ctx context.Context, w Writer, description string, cat Catalog) []string {
	out := []string{}
	if w == nil || strings.TrimSpace(description) == "" {
		return out
	}
	var b strings.Builder
	b.WriteString("Look of the video:\n" + description + "\n\nCatalog:\n")
	for _, p := range cat.List() {
		if !p.Manual {
			fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Description)
		}
	}
	answer, err := w.Write(ctx, fmt.Sprintf(presetPartsSystem, MaxPresetParts), b.String(), 300)
	if err != nil {
		return out
	}
	raw := extractJSON(answer)
	if raw == "" {
		return out
	}
	var got struct {
		Parts []string `json:"parts"`
	}
	if json.Unmarshal([]byte(raw), &got) != nil {
		return out
	}
	seen := map[string]bool{}
	for _, n := range got.Parts {
		n = strings.TrimSpace(n)
		p, ok := cat.Get(n)
		if !ok || p.Manual || seen[n] || len(out) >= MaxPresetParts {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
