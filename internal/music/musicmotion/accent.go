package musicmotion

// accent.go: fixing an accent that has no contrast instead of asking a model for it a third time. Only the lightness moves, in the
// OKLCH space (where lightness is perceptual and a change of it does not turn the hue), so the colour stays the one the assistant
// chose, a little darker or lighter.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	// accentStep is how far the lightness (0 to 1 in OKLCH) moves at a time: 2 %.
	accentStep = 0.02
	// accentMaxShift is the most it moves in total: 40 %.
	accentMaxShift = 0.40
)

// usedModes are the modes some section of the design is drawn in.
func (d Design) usedModes() []string {
	set := map[string]bool{}
	if d.DefaultMode == ModeLight {
		set[ModeLight] = true
	} else {
		set[ModeDark] = true
	}
	for _, m := range d.Modes {
		if m == ModeDark || m == ModeLight {
			set[m] = true
		}
	}
	var out []string
	for m := range set {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// AdjustAccent moves the lightness of the accent of a design, keeping its hue and its chroma (as far as the colour fits in sRGB),
// until it has at least MinAccentContrast against the background of every mode the design uses. It looks both ways in steps of 2 %
// up to 40 % and takes the shorter move. The note says what was done ("accent #d48806 adjusted to #8f5a00 for 3:1 contrast on the
// light mode"). ok is false, with the design as it was, when the accent already passes or when no lightness within reach does (two
// modes in use whose backgrounds leave no room between them, for one).
func AdjustAccent(d Design) (Design, string, bool) {
	if !hexColour.MatchString(d.Accent) || !hexColour.MatchString(d.Dark) || !hexColour.MatchString(d.Light) {
		return d, "", false
	}
	modes := d.usedModes()
	var failing []string
	passes := func(accent string) bool {
		for _, m := range modes {
			bg, _, _ := d.Colours(m)
			if Contrast(accent, bg) < MinAccentContrast {
				return false
			}
		}
		return true
	}
	for _, m := range modes {
		bg, _, _ := d.Colours(m)
		if Contrast(d.Accent, bg) < MinAccentContrast {
			failing = append(failing, m)
		}
	}
	if len(failing) == 0 {
		return d, "", false
	}
	l, c, h := hexToOKLCH(d.Accent)
	try := func(dir float64) (string, int) {
		for k := 1; float64(k)*accentStep <= accentMaxShift+1e-9; k++ {
			nl := l + dir*float64(k)*accentStep
			if nl < 0 || nl > 1 {
				break
			}
			if hex := okLCHToHex(nl, c, h); passes(hex) {
				return hex, k
			}
		}
		return "", 0
	}
	darker, kd := try(-1)
	lighter, kl := try(+1)
	var pick string
	switch {
	case darker == "" && lighter == "":
		return d, "", false
	case lighter == "":
		pick = darker
	case darker == "":
		pick = lighter
	case kd < kl:
		pick = darker
	case kl < kd:
		pick = lighter
	default: // the same distance: away from the first background that was too close
		bg, _, _ := d.Colours(failing[0])
		la, _ := luminance(d.Accent)
		lb, _ := luminance(bg)
		if la > lb {
			pick = lighter
		} else {
			pick = darker
		}
	}
	out := d
	out.Accent = pick
	note := fmt.Sprintf("accent %s adjusted to %s for %.0f:1 contrast on the %s mode", d.Accent, pick, MinAccentContrast, strings.Join(failing, " and "))
	if len(failing) > 1 {
		note += "s"
	}
	return out, note, true
}

// --- OKLCH ------------------------------------------------------------------------------------

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSRGB(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func hexToRGB(hex string) (r, g, b float64) {
	h := hex[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		return float64(v) / 255
	}
	return ch(0), ch(1), ch(2)
}

// hexToOKLCH is the lightness (0 to 1), chroma and hue (radians) of a hex colour.
func hexToOKLCH(hex string) (l, c, h float64) {
	r, g, b := hexToRGB(hex)
	r, g, b = srgbToLinear(r), srgbToLinear(g), srgbToLinear(b)
	lm := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mm := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sm := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*lm + 0.7936177850*mm - 0.0040720468*sm
	a := 1.9779984951*lm - 2.4285922050*mm + 0.4505937099*sm
	bb := 0.0259040371*lm + 0.7827717662*mm - 0.8086757660*sm
	return L, math.Hypot(a, bb), math.Atan2(bb, a)
}

// oklchToLinear is the linear sRGB of an OKLCH colour, unclamped.
func oklchToLinear(l, c, h float64) (r, g, b float64) {
	a, bb := c*math.Cos(h), c*math.Sin(h)
	lm := l + 0.3963377774*a + 0.2158037573*bb
	mm := l - 0.1055613458*a - 0.0638541728*bb
	sm := l - 0.0894841775*a - 1.2914855480*bb
	lm, mm, sm = lm*lm*lm, mm*mm*mm, sm*sm*sm
	return 4.0767416621*lm - 3.3077115913*mm + 0.2309699292*sm,
		-1.2684380046*lm + 2.6097574011*mm - 0.3413193965*sm,
		-0.0041960863*lm - 0.7034186147*mm + 1.7076147010*sm
}

// okLCHToHex is the hex colour of an OKLCH colour; when the chroma does not fit in sRGB at that lightness it is lowered (the
// lightness and the hue are what the caller asked for).
func okLCHToHex(l, c, h float64) string {
	inGamut := func(c float64) bool {
		r, g, b := oklchToLinear(l, c, h)
		const eps = 1e-6
		return r >= -eps && r <= 1+eps && g >= -eps && g <= 1+eps && b >= -eps && b <= 1+eps
	}
	if !inGamut(c) {
		lo, hi := 0.0, c
		for i := 0; i < 24; i++ {
			mid := (lo + hi) / 2
			if inGamut(mid) {
				lo = mid
			} else {
				hi = mid
			}
		}
		c = lo
	}
	r, g, b := oklchToLinear(l, c, h)
	to8 := func(v float64) int {
		v = math.Min(1, math.Max(0, v))
		return int(math.Round(linearToSRGB(v) * 255))
	}
	return fmt.Sprintf("#%02x%02x%02x", to8(r), to8(g), to8(b))
}
