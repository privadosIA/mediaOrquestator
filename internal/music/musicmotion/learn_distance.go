package musicmotion

// learn_distance.go: how different a style or a part is from what exists (spec motion-learning, "A learning proposes what
// is different"). The measure is a number from 0 (the same) to 1 (nothing in common) with a short reason, computed
// without a model so that the same input gives the same answer. A proposal whose distance to the closest thing that exists
// is below the threshold of the strictness asked is "too similar".

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Strictness says how far from what exists a proposal has to be to count as new.
type Strictness string

// The three strictness levels.
const (
	StrictnessLow    Strictness = "low"
	StrictnessNormal Strictness = "normal"
	StrictnessHigh   Strictness = "high"
)

// Distances below which a proposal is too similar, for each strictness. Low <= normal <= high always holds, so the
// same proposal discarded under low is discarded under high.
const (
	thresholdLow    = 0.15
	thresholdNormal = 0.25
	thresholdHigh   = 0.40
)

// ParseStrictness reads the strictness of a request ("" is normal).
func ParseStrictness(s string) (Strictness, error) {
	switch Strictness(strings.ToLower(strings.TrimSpace(s))) {
	case "", StrictnessNormal:
		return StrictnessNormal, nil
	case StrictnessLow:
		return StrictnessLow, nil
	case StrictnessHigh:
		return StrictnessHigh, nil
	}
	return "", fmt.Errorf("strictness must be low, normal or high, got %q", s)
}

// Threshold is the distance below which something is too similar to what exists.
func (s Strictness) Threshold() float64 {
	switch s {
	case StrictnessLow:
		return thresholdLow
	case StrictnessHigh:
		return thresholdHigh
	}
	return thresholdNormal
}

// Similar names the closest thing that exists to a proposal.
type Similar struct {
	// Kind is "style", "part" (of the catalog) or "draft" (a part that is not approved yet).
	Kind     string  `json:"kind"`
	ID       string  `json:"id,omitempty"`
	Name     string  `json:"name"`
	Distance float64 `json:"distance"`
	Reason   string  `json:"reason"`
}

// TooSimilar says whether a proposal is not new for the strictness: its closest neighbour is nearer than the threshold.
func TooSimilar(s Similar, st Strictness) bool { return s.Distance < st.Threshold() }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// hsl reads a #rgb or #rrggbb colour as hue (0 to 1), saturation and lightness; ok is false when it is not a colour.
func hsl(hex string) (h, s, l float64, ok bool) {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return 0, 0, 0, false
	}
	r, g, b = r/255, g/255, b/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l, true
	}
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
	return h / 6, s, l, true
}

// colourDistance is how far apart two colours look: the hue counts as much as the colours have of it (a grey has none),
// the saturation and the lightness count on their own. A colour that cannot be read is as far as can be.
func colourDistance(a, b string) float64 {
	h1, s1, l1, ok1 := hsl(a)
	h2, s2, l2, ok2 := hsl(b)
	if !ok1 || !ok2 {
		return 1
	}
	dh := math.Abs(h1 - h2)
	if dh > 0.5 {
		dh = 1 - dh
	}
	dh *= 2 // 0 to 1
	return clamp01(0.5*dh*math.Min(s1, s2)*2 + 0.25*math.Abs(s1-s2) + 0.25*math.Abs(l1-l2)*2)
}

func plainIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "plain"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// partsDistance is one minus the overlap of two lists of parts; an empty list means "all the usual ones", so it is
// halfway from any other list and equal to another empty list.
func partsDistance(a, b []string) float64 {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0 || len(b) == 0:
		return 0.5
	}
	in := map[string]bool{}
	for _, n := range a {
		in[n] = true
	}
	both, all := 0, len(in)
	for _, n := range b {
		if in[n] {
			both++
		} else {
			all++
		}
	}
	return 1 - float64(both)/float64(all)
}

type component struct {
	name   string
	weight float64
	value  float64
}

// describeComponents says what makes two things differ (the ones that count) or that they agree.
func describeComponents(cs []component, same string) string {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].weight*cs[i].value > cs[j].weight*cs[j].value })
	var parts []string
	for _, c := range cs {
		if c.value >= 0.1 {
			parts = append(parts, fmt.Sprintf("%s (%.2f)", c.name, c.value))
		}
	}
	if len(parts) == 0 {
		return same
	}
	return "differs in " + strings.Join(parts, ", ")
}

// StyleDistance measures how different two styles are: the accent, the dark tone, the rhythm, the finishes of the text
// and the parts they use (weights 0.40, 0.15, 0.15, 0.15 and 0.15).
func StyleDistance(a, b Preset) (float64, string) {
	rhythm := 0.0
	if m := math.Max(a.Rhythm, b.Rhythm); m > 0 {
		rhythm = math.Abs(a.Rhythm-b.Rhythm) / m
	}
	finishes := 0.0
	for _, pair := range [][2]string{
		{strings.ToLower(a.Design.Display), strings.ToLower(b.Design.Display)},
		{strings.ToLower(a.Design.Body), strings.ToLower(b.Design.Body)},
		{plainIfEmpty(a.Design.VerseStyle), plainIfEmpty(b.Design.VerseStyle)},
		{plainIfEmpty(a.Design.HitStyle), plainIfEmpty(b.Design.HitStyle)},
	} {
		if pair[0] != pair[1] {
			finishes += 0.25
		}
	}
	cs := []component{
		{"accent", 0.40, colourDistance(a.Design.Accent, b.Design.Accent)},
		{"dark tone", 0.15, colourDistance(a.Design.Dark, b.Design.Dark)},
		{"rhythm", 0.15, clamp01(rhythm)},
		{"finishes", 0.15, finishes},
		{"parts", 0.15, partsDistance(a.Parts, b.Parts)},
	}
	total := 0.0
	for _, c := range cs {
		total += c.weight * c.value
	}
	return math.Round(clamp01(total)*1000) / 1000, describeComponents(cs, "same accent, tones, rhythm, finishes and parts")
}

// PartLike is a part (of the catalog or a draft), or the idea of one: what is compared of it.
type PartLike struct {
	Kind        string // "part" or "draft"
	ID          string
	Name        string
	Description string
}

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the of and or with on in to it its is are as at by for from that this each every one into over
		then when while per via no not up out all any can will shows show draws draw`) {
		stopWords[w] = true
	}
}

// words are the lower-case words of a text that say something (no short words and no filler), without repeats.
func words(texts ...string) map[string]bool {
	out := map[string]bool{}
	for _, t := range texts {
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len(w) >= 3 && !stopWords[w] {
				out[w] = true
			}
		}
	}
	return out
}

// PartDistance measures how different two parts are by the words of their name and description: one minus the share of
// the words they have in common. The same name is distance 0.
func PartDistance(a, b PartLike) (float64, string) {
	if strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name)) && strings.TrimSpace(a.Name) != "" {
		return 0, "the same name"
	}
	wa, wb := words(a.Name, a.Description), words(b.Name, b.Description)
	if len(wa) == 0 || len(wb) == 0 {
		return 1, "nothing to compare"
	}
	both := 0
	for w := range wa {
		if wb[w] {
			both++
		}
	}
	union := len(wa) + len(wb) - both
	d := math.Round((1-float64(both)/float64(union))*1000) / 1000
	return d, fmt.Sprintf("shares %d of %d words", both, union)
}

// ClosestStyle finds the style of existing nearest to c; ok is false when there are none.
func ClosestStyle(c Preset, existing []Preset) (Similar, bool) {
	best, found := Similar{}, false
	for _, e := range existing {
		d, why := StyleDistance(c, e)
		if !found || d < best.Distance {
			best, found = Similar{Kind: "style", ID: e.ID, Name: e.Name, Distance: d, Reason: why}, true
		}
	}
	return best, found
}

// ClosestPart finds the part of existing nearest to c; ok is false when there are none.
func ClosestPart(c PartLike, existing []PartLike) (Similar, bool) {
	best, found := Similar{}, false
	for _, e := range existing {
		d, why := PartDistance(c, e)
		if !found || d < best.Distance {
			kind := e.Kind
			if kind == "" {
				kind = "part"
			}
			best, found = Similar{Kind: kind, ID: e.ID, Name: e.Name, Distance: d, Reason: why}, true
		}
	}
	return best, found
}
