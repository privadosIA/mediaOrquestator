package musicmotion

// scenerepair.go: what a program can fix in the answer of the assistant before the checks refuse it (spec motion-scene-parts,
// "A scene draft is repaired before it is refused"). The checks of scenechecks.go and checkpart.go send a slip back to the model and
// the model often makes a different one; the slips that are mechanical are repaired here: a colour that is a literal instead of one
// of the design's variables, a font of its own, and Math.random(). Nothing else is touched, and a fragment that needs no repair
// comes out byte for byte.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Repair says what was repaired in a draft: its kind (RepairColour, RepairFont or RepairClock) and in how many places.
type Repair struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// The kinds of repair.
const (
	RepairColour = "colour"
	RepairFont   = "font"
	RepairClock  = "clock"
	RepairTypo   = "typo" // var() written with letters more (varvare(--fg)) or a </style> closed with a typo (</sc>)
)

// seededRandom replaces Math.random() in a scene: the same numbers in every render, so the frames land the same.
const seededRandom = "var __seed=1234567;function __rnd(){__seed=(__seed*1664525+1013904223)>>>0;return __seed/4294967296}"

var (
	// reDecl is a property and its value; the first group is what comes before the value, the second the property, the third the value.
	reDecl = regexp.MustCompile(`(?i)((?:^|[;{\s])(--[\w-]+|[a-z-]+)\s*:\s*)([^;{}]*)`)
	// reRepairColour finds, in a value, what is left alone (var(), url()) and what is a colour a program can read: a function with no
	// nesting, a hex, black or white.
	reRepairColour = regexp.MustCompile(`(?i)var\([^)]*\)|url\([^)]*\)|(?:rgba?|hsla?)\([^()]*\)|#[0-9a-f]{3,8}\b|\b(?:black|white)\b`)
	reMathRandom   = regexp.MustCompile(`Math\.random\(\)`)
	// reVarTypo is var() written with letters more: varvare(--fg), varine(--display).
	reVarTypo = regexp.MustCompile(`(?i)\bvar[a-z]+\(\s*--`)
	// reStyleTypo is a closing tag that is a typo of </style> (</sc>, </sty>, </styl>).
	reStyleTypo   = regexp.MustCompile(`(?i)</(?:sc|sty|styl|stlye|stile|stye|syle)\s*>`)
	reStyleOpen   = regexp.MustCompile(`(?i)<style[\s>]`)
	reStyleClose  = regexp.MustCompile(`(?i)</style\s*>`)
	reFirstScript = regexp.MustCompile(`(?i)<script[^>]*>`)
)

type rgb struct{ r, g, b float64 }

// RepairScene repairs the slips of a scene fragment that the checks refuse and a program can fix, against the design the scene is
// drawn with, and lists what it did. A fragment with none comes back unchanged.
func RepairScene(fragment string, d Design) (string, []Repair) {
	toks := designTokens(d)
	var colours, fonts, typos int
	if reStyleTypo.MatchString(fragment) && len(reStyleOpen.FindAllString(fragment, -1)) > len(reStyleClose.FindAllString(fragment, -1)) {
		fragment = reStyleTypo.ReplaceAllStringFunc(fragment, func(string) string { typos++; return "</style>" })
	}
	if n := len(reVarTypo.FindAllString(fragment, -1)); n > 0 {
		fragment = reVarTypo.ReplaceAllString(fragment, "var(--")
		typos += n
	}
	fix := func(prop, value string) string {
		nv, c, f := repairValue(strings.ToLower(prop), value, toks)
		colours, fonts = colours+c, fonts+f
		return nv
	}
	repairDecls := func(css string) string {
		return reDecl.ReplaceAllStringFunc(css, func(m string) string {
			g := reDecl.FindStringSubmatch(m)
			return g[1] + fix(g[2], g[3])
		})
	}
	out := reSceneStyle.ReplaceAllStringFunc(fragment, func(m string) string {
		g := reSceneStyle.FindStringSubmatch(m)
		return strings.Replace(m, g[1], repairDecls(g[1]), 1)
	})
	out = reStyleAttr.ReplaceAllStringFunc(out, func(m string) string {
		g := reStyleAttr.FindStringSubmatch(m)
		body := g[1] + g[2]
		return strings.Replace(m, body, repairDecls(body), 1)
	})
	out = reSVGColorAtt.ReplaceAllStringFunc(out, func(m string) string {
		g := reSVGColorAtt.FindStringSubmatch(m)
		val := g[2] + g[3]
		return strings.Replace(m, val, fix(g[1], val), 1)
	})
	out = reSVGFontAtt.ReplaceAllStringFunc(out, func(m string) string {
		g := reSVGFontAtt.FindStringSubmatch(m)
		val := g[2] + g[3]
		return strings.Replace(m, val, fix(g[1], val), 1)
	})
	var reps []Repair
	if typos > 0 {
		reps = append(reps, Repair{Kind: RepairTypo, Count: typos})
	}
	if colours > 0 {
		reps = append(reps, Repair{Kind: RepairColour, Count: colours})
	}
	if fonts > 0 {
		reps = append(reps, Repair{Kind: RepairFont, Count: fonts})
	}
	if n := len(reMathRandom.FindAllString(out, -1)); n > 0 {
		if loc := reFirstScript.FindStringIndex(out); loc != nil {
			out = out[:loc[1]] + seededRandom + ";" + reMathRandom.ReplaceAllString(out[loc[1]:], "__rnd()")
			reps = append(reps, Repair{Kind: RepairClock, Count: n})
		}
	}
	return out, reps
}

// designTokens are the colours the scene's variables hold in the mode the design starts in: --bg, --fg and --accent.
func designTokens(d Design) map[string]rgb {
	dark, light := parseColour(d.Dark), parseColour(d.Light)
	bg, fg := dark, light
	if d.DefaultMode == ModeLight {
		bg, fg = light, dark
	}
	return map[string]rgb{"--bg": bg.rgb, "--fg": fg.rgb, "--accent": parseColour(d.Accent).rgb}
}

type colour struct {
	rgb
	a  float64
	ok bool
}

// repairValue repairs one declaration: the colour literals of a colour property and the font of a font-family. It says how many
// colours and fonts it changed. A value it cannot read is left as it is.
func repairValue(prop, value string, toks map[string]rgb) (string, int, int) {
	switch {
	case prop == "font-family":
		if fontProblem(prop, value, "") == "" {
			return value, 0, 0
		}
		return "var(--display)", 0, 1
	case colourProps[prop] || strings.HasPrefix(prop, "--"):
		if colourLiteral(withoutNeutralShadows(prop, value)) == "" {
			return value, 0, 0
		}
		n := 0
		nv := reRepairColour.ReplaceAllStringFunc(value, func(tok string) string {
			low := strings.ToLower(tok)
			if strings.HasPrefix(low, "var(") || strings.HasPrefix(low, "url(") {
				return tok
			}
			if (prop == "box-shadow" || prop == "text-shadow") && reNeutralColour.MatchString(tok) {
				return tok // a shadow of black or white is allowed by the check
			}
			c := parseColour(tok)
			if !c.ok {
				return tok
			}
			n++
			return tokenFor(c, toks)
		})
		return nv, n, 0
	}
	return value, 0, 0
}

// tokenFor is the variable closest to a colour (var(--bg), var(--fg) or var(--accent)), and a color-mix() of it for a colour that is not opaque.
func tokenFor(c colour, toks map[string]rgb) string {
	best, bd := "--fg", math.MaxFloat64
	for _, name := range []string{"--bg", "--fg", "--accent"} {
		t := toks[name]
		if d := (t.r-c.r)*(t.r-c.r) + (t.g-c.g)*(t.g-c.g) + (t.b-c.b)*(t.b-c.b); d < bd {
			best, bd = name, d
		}
	}
	if c.a >= 0.995 {
		return "var(" + best + ")"
	}
	return fmt.Sprintf("color-mix(in srgb, var(%s) %d%%, transparent)", best, int(math.Round(c.a*100)))
}

// parseColour reads a hex (3, 4, 6 or 8 digits), rgb()/rgba(), hsl()/hsla(), black or white; ok is false for anything else.
func parseColour(s string) colour {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "black":
		return colour{rgb{0, 0, 0}, 1, true}
	case s == "white":
		return colour{rgb{255, 255, 255}, 1, true}
	case strings.HasPrefix(s, "#"):
		h := s[1:]
		if len(h) == 3 || len(h) == 4 {
			var e string
			for _, r := range h {
				e += string(r) + string(r)
			}
			h = e
		}
		if len(h) != 6 && len(h) != 8 {
			return colour{}
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return colour{}
		}
		if len(h) == 8 {
			return colour{rgb{float64(v >> 24 & 255), float64(v >> 16 & 255), float64(v >> 8 & 255)}, float64(v&255) / 255, true}
		}
		return colour{rgb{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)}, 1, true}
	case strings.HasPrefix(s, "rgb"), strings.HasPrefix(s, "hsl"):
		open := strings.Index(s, "(")
		if open < 0 || !strings.HasSuffix(s, ")") {
			return colour{}
		}
		f := strings.FieldsFunc(s[open+1:len(s)-1], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(f) < 3 || len(f) > 4 {
			return colour{}
		}
		a := 1.0
		if len(f) == 4 {
			v, pct, ok := number(f[3])
			if !ok {
				return colour{}
			}
			a = v
			if pct {
				a = v / 100
			}
		}
		n := make([]float64, 3)
		for i := 0; i < 3; i++ {
			v, pct, ok := number(f[i])
			if !ok {
				return colour{}
			}
			if strings.HasPrefix(s, "rgb") && pct {
				v = v * 255 / 100
			}
			n[i] = v
		}
		if strings.HasPrefix(s, "hsl") {
			r, g, b := hslToRGB(n[0], n[1]/100, n[2]/100)
			return colour{rgb{r, g, b}, math.Min(1, math.Max(0, a)), true}
		}
		return colour{rgb{n[0], n[1], n[2]}, math.Min(1, math.Max(0, a)), true}
	}
	return colour{}
}

// number reads 12, 0.5, 40% or 90deg: its value, whether it had a % and whether it is a number.
func number(s string) (float64, bool, bool) {
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "%"), "deg")
	v, err := strconv.ParseFloat(s, 64)
	return v, pct, err == nil
}

func hslToRGB(h, s, l float64) (float64, float64, float64) {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	if s == 0 {
		return l * 255, l * 255, l * 255
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		t = math.Mod(t+1, 1)
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return hue(h+1.0/3) * 255, hue(h) * 255, hue(h-1.0/3) * 255
}
