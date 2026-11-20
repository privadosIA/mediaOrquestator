package musicmotion

// theme.go: the theme of a design (spec motion-house-style): a stylesheet, a few small SVG assets and the four texts of a HUD,
// written once and carried by the design (so by presets and datasets) into every page of a piece.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Limits of a theme.
const (
	MaxThemeCSS    = 20 << 10
	MaxThemeAssets = 5
	MaxThemeAsset  = 50 << 10
	MaxHUDText     = 40
)

// HUD are the four corner labels of the hud-chrome part: top left, top right, bottom left and bottom right.
type HUD struct {
	TL string `json:"tl,omitempty"`
	TR string `json:"tr,omitempty"`
	BL string `json:"bl,omitempty"`
	BR string `json:"br,omitempty"`
}

// Theme is the look a piece shares across its pages.
type Theme struct {
	// CSS is injected after the page's own stylesheet in every page: it may restyle #verse and #hit, add @font-face from data
	// URLs and define the glow and the type scale.
	CSS string `json:"css,omitempty"`
	// Assets are small SVG drawings (a mascot, a logo) a part reads as window.__theme.assets[name] or places with {{asset:name}}.
	Assets map[string]string `json:"assets,omitempty"`
	// HUD feeds the hud-chrome part.
	HUD *HUD `json:"hud,omitempty"`
}

var (
	reAssetName   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	reCSSForbid   = regexp.MustCompile(`(?i)@import|expression\s*\(|behavior\s*:|javascript\s*:|<`)
	reURLCall     = regexp.MustCompile(`(?i)url\s*\(\s*["']?\s*([^"')\s]{0,5})`)
	reAssetSlot   = regexp.MustCompile(`\{\{\s*asset:([a-z][a-z0-9_]*)\s*\}\}`)
	reAssetSlotNo = regexp.MustCompile(`\{\{\s*asset:([^}\s]*)\s*\}\}`)
)

// Validate checks a theme and names the first field that is wrong.
func (t Theme) Validate() error {
	if len(t.CSS) > MaxThemeCSS {
		return fmt.Errorf("design.theme.css is %d bytes, the most is %d", len(t.CSS), MaxThemeCSS)
	}
	if m := reCSSForbid.FindString(t.CSS); m != "" {
		return fmt.Errorf("design.theme.css has %q, which a theme may not have (no @import, expression(), behavior:, javascript: or <)", strings.TrimSpace(m))
	}
	for _, m := range reURLCall.FindAllStringSubmatch(t.CSS, -1) {
		if !strings.HasPrefix(strings.ToLower(m[1]), "data:") {
			return fmt.Errorf("design.theme.css has a url() that is not a data: URL: everything must be inline")
		}
	}
	if len(t.Assets) > MaxThemeAssets {
		return fmt.Errorf("design.theme.assets has %d drawings, the most is %d", len(t.Assets), MaxThemeAssets)
	}
	names := make([]string, 0, len(t.Assets))
	for n := range t.Assets {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		svg := t.Assets[n]
		switch {
		case !reAssetName.MatchString(n):
			return fmt.Errorf("design.theme.assets[%q]: the name must be lower case letters, digits and underscores", n)
		case len(svg) > MaxThemeAsset:
			return fmt.Errorf("design.theme.assets[%q] is %d bytes, the most is %d", n, len(svg), MaxThemeAsset)
		}
		if p := svgProblem(svg); p != "" {
			return fmt.Errorf("design.theme.assets[%q]: %s", n, p)
		}
	}
	if t.HUD != nil {
		for name, v := range map[string]string{"tl": t.HUD.TL, "tr": t.HUD.TR, "bl": t.HUD.BL, "br": t.HUD.BR} {
			if n := len([]rune(v)); n > MaxHUDText {
				return fmt.Errorf("design.theme.hud.%s has %d characters, the most is %d", name, n, MaxHUDText)
			}
		}
	}
	return nil
}

// withHUD is the theme with its hud replaced (the theme itself is not changed).
func (t *Theme) withHUD(h *HUD) *Theme {
	var out Theme
	if t != nil {
		out = *t
	}
	cp := *h
	out.HUD = &cp
	return &out
}

// fillAssets puts the theme's drawings in the {{asset:name}} slots of a fragment; a name the theme does not have leaves the slot
// empty.
func fillAssets(frag string, t *Theme) string {
	return reAssetSlot.ReplaceAllStringFunc(frag, func(m string) string {
		if t == nil {
			return ""
		}
		return t.Assets[reAssetSlot.FindStringSubmatch(m)[1]]
	})
}

// UnknownAssets lists the {{asset:name}} slots of a fragment that the theme has no drawing for.
func UnknownAssets(frag string, t *Theme) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range reAssetSlotNo.FindAllStringSubmatch(frag, -1) {
		name := m[1]
		if _, ok := themeAsset(t, name); !ok && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func themeAsset(t *Theme, name string) (string, bool) {
	if t == nil {
		return "", false
	}
	v, ok := t.Assets[name]
	return v, ok
}
