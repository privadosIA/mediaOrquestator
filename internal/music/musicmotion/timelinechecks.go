package musicmotion

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// The rules of the timeline contract (spec music-motion-parts): what can be read in the text of the fragment is checked in
// timelineProblems, what the timelines do to the page in timelinePageProblems (the runtime's problems function).

var reForeverRepeat = regexp.MustCompile(`repeat\s*:\s*-\s*1\b`)

// gsapPlugins are the plugins of GSAP that are not in the core file the page embeds; a fragment that calls one cannot run.
const gsapPlugins = `SplitText|ScrollTrigger|ScrollSmoother|ScrollToPlugin|Flip|Draggable|Observer|MotionPathPlugin|MotionPathHelper|MorphSVGPlugin|DrawSVGPlugin|ScrambleTextPlugin|TextPlugin|CustomEase|CustomBounce|CustomWiggle|InertiaPlugin|Physics2DPlugin|PhysicsPropsPlugin|PixiPlugin|EaselPlugin|GSDevTools|RoughEase|SlowMo|ExpoScaleEase|CSSRulePlugin`

// reGSAPPlugin finds a plugin used as code (registered, instantiated or called), not the same word inside a text.
var reGSAPPlugin = regexp.MustCompile(`registerPlugin\([^)]*\b(` + gsapPlugins + `)\b|\bnew\s+(` + gsapPlugins + `)\b|\b(` + gsapPlugins + `)\s*\.\s*\w+\s*\(`)

// timelineProblems are what the text of a fragment that uses GSAP or clips says is wrong.
func timelineProblems(frag string) []string {
	if !usesTimelines(frag) {
		return nil
	}
	var out []string
	if reForeverRepeat.MatchString(frag) {
		out = append(out, "a timeline repeats forever (repeat: -1): the page seeks every timeline to the time of the frame, so an endless repeat is not needed and it hides a part that only loops; give it a finite number of repeats or build the motion over the length of the part")
	}
	if usesGSAP(frag) {
		if m := reGSAPPlugin.FindStringSubmatch(frag); m != nil {
			name := m[1] + m[2] + m[3]
			out = append(out, fmt.Sprintf("it uses the GSAP plugin %s, which is not embedded (only the GSAP core is, with no plugins): split the text into elements yourself and tween them with gsap.to, gsap.from or gsap.fromTo", name))
		}
	}
	return out
}

// timelinePageProblems draws the part's page and asks the timeline runtime what its timelines do: a repeat that never ends, a
// tween of display, visibility or autoAlpha on a clip, a CSS transform on an element a timeline moves. A clip that ends after the
// scene is only warned about (logged).
func timelinePageProblems(ctx context.Context, browser string, p Part, sc Scene) ([]string, error) {
	if !usesTimelines(p.Fragment) {
		return nil, nil
	}
	s, _, err := openCheckSession(ctx, browser, sc)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if _, err := s.Frame(ctx, 0.5); err != nil {
		return nil, err
	}
	var res struct {
		Problems []string `json:"problems"`
		Warnings []string `json:"warnings"`
	}
	if err := s.EvalJSON(ctx, fmt.Sprintf("JSON.stringify(window.__hf.problems(%v))", sc.Duration), &res); err != nil {
		return nil, err
	}
	for _, w := range res.Warnings {
		slog.Warn("a clip of the part is longer than the scene", "part", p.Name, "warning", w)
	}
	return res.Problems, nil
}

// Timeline says the part animates with a registered GSAP timeline.
func (p Part) Timeline() bool { return strings.Contains(p.Fragment, "__timelines") }
