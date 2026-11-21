package motion

// revise.go: change the motion of a page that already exists without writing the page again. The model is shown the page
// (through a Viewer, which may leave the paint out) and the request, and answers with the JSON of the page's timeline
// block only. The block is checked, spliced into the stored page, and the result goes through the same check as a new
// page. The paint of the page never goes through the model, so the model cannot change it; and the answer is a few
// hundred tokens, never a whole page.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// reviseMaxTokens bounds the answer: a timeline is a few hundred tokens, never a page.
const reviseMaxTokens = 2000

// ErrNoTimeline is returned when the page has no motion-timeline block to revise.
var ErrNoTimeline = errors.New("the page has no motion-timeline block to revise")

// ErrTimelineRejected is returned when the model could not write a timeline that passes the check.
var ErrTimelineRejected = errors.New("the model could not write a working timeline")

// Viewer returns what the model is shown of a page. It may leave out what the model does not need (a compression
// service that drops the paint, for example) but it must keep the timeline block; when it does not, the model is shown
// the page itself.
type Viewer func(ctx context.Context, page string) string

var reTimeline = regexp.MustCompile(`(?is)(<script\b[^>]*\bid\s*=\s*["']motion-timeline["'][^>]*>)(.*?)(</script\s*>)`)

// ExtractTimeline returns the content of the page's motion-timeline block.
func ExtractTimeline(page string) (string, bool) {
	m := reTimeline.FindStringSubmatch(page)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[2]), true
}

// SpliceTimeline replaces the content of the page's motion-timeline block and leaves every other byte as it was. It
// reports false when the page has no such block.
func SpliceTimeline(page, timeline string) (string, bool) {
	loc := reTimeline.FindStringSubmatchIndex(page)
	if loc == nil {
		return "", false
	}
	return page[:loc[4]] + timeline + page[loc[5]:], true
}

var reFence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// ExtractTimelineJSON takes the JSON object out of a model's answer: inside a markdown fence when there is one, else from
// the first { to the last }. A whole <script> element is accepted too.
func ExtractTimelineJSON(answer string) string {
	s := answer
	if m := reFence.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if inner, ok := ExtractTimeline(s); ok {
		s = inner
	}
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(s[i : j+1])
}

// timelineRanges are the values each property of a track may take: the same as RANGES in timelineScript, and a test
// keeps the two equal.
var timelineRanges = map[string][2]float64{
	"opacity": {0, 1}, "x": {-1e5, 1e5}, "y": {-1e5, 1e5}, "scale": {0, 1e3},
	"scaleX": {0, 1e3}, "rotate": {-1e5, 1e5}, "blur": {0, 1e3}, "reveal": {0, 1},
}

func asNumber(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

// ValidateTimeline checks the shape of a timeline the way the page's own check does (see __motionTimelineProblems in
// timelineScript), except for what needs the page itself: whether a selector matches an element and whether a track
// ends after the clip. The browser check that every page goes through still does both. It returns the problems, none
// when the timeline is fine.
func ValidateTimeline(raw string) []string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return []string{"the answer is not valid JSON: " + err.Error()}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return []string{`the timeline must be a JSON object {"version": 1, "tracks": [...]}`}
	}
	var out []string
	if n, ok := asNumber(obj["version"]); !ok || n != 1 {
		out = append(out, `the timeline "version" must be 1`)
	}
	tracks, ok := obj["tracks"].([]any)
	if !ok {
		return append(out, `the timeline needs a "tracks" array`)
	}
	ids := map[string]bool{}
	for i, t := range tracks {
		name := fmt.Sprintf("track %d", i+1)
		tr, ok := t.(map[string]any)
		if !ok {
			out = append(out, name+" is not an object")
			continue
		}
		if id, _ := tr["id"].(string); id == "" {
			out = append(out, name+` has no "id"`)
		} else if ids[id] {
			out = append(out, name+" repeats an id")
		} else {
			ids[id] = true
			name = fmt.Sprintf("track %d (%s)", i+1, id)
		}
		if sel, _ := tr["target"].(string); sel == "" {
			out = append(out, name+` has no "target" selector`)
		}
		if s, ok := asNumber(tr["start"]); !ok || s < 0 {
			out = append(out, name+`: "start" must be a number of seconds, 0 or more`)
		}
		if d, ok := asNumber(tr["duration"]); !ok || d <= 0 {
			out = append(out, name+`: "duration" must be a number of seconds, more than 0`)
		}
		if sg, has := tr["stagger"]; has {
			if f, ok := asNumber(sg); !ok || f < 0 {
				out = append(out, name+`: "stagger" must be a number of seconds, 0 or more`)
			}
		}
		if e, has := tr["ease"]; has && !validCurve(e) {
			out = append(out, name+`: "ease" must be one of `+strings.Join(TimelineCurves, ", ")+" or four numbers of a cubic-bezier")
		}
		for _, side := range []string{"from", "to"} {
			o, ok := tr[side].(map[string]any)
			if !ok {
				out = append(out, name+` has no "`+side+`" object`)
				continue
			}
			for _, k := range sortedKeys(o) {
				rng, known := timelineRanges[k]
				f, isNum := asNumber(o[k])
				switch {
				case !known:
					out = append(out, fmt.Sprintf(`%s: "%s" in "%s" is not a property (use %s)`, name, k, side, strings.Join(TimelineProperties, ", ")))
				case !isNum:
					out = append(out, fmt.Sprintf(`%s: "%s" in "%s" must be a number`, name, k, side))
				case f < rng[0] || f > rng[1]:
					out = append(out, fmt.Sprintf(`%s: "%s" in "%s" is out of range (%g to %g)`, name, k, side, rng[0], rng[1]))
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(raw), "</script") {
		out = append(out, `the timeline must not contain "</script"`)
	}
	return out
}

func validCurve(e any) bool {
	switch c := e.(type) {
	case string:
		for _, name := range TimelineCurves {
			if c == name {
				return true
			}
		}
	case []any:
		if len(c) != 4 {
			return false
		}
		for _, n := range c {
			if _, ok := asNumber(n); !ok {
				return false
			}
		}
		return true
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func compactJSON(s string) string {
	var b bytes.Buffer
	if json.Compact(&b, []byte(s)) != nil {
		return s
	}
	return b.String()
}

// reviseSystemPrompt asks for the timeline only. The properties, their ranges and the curves come from the same data the
// checks use, so the instructions cannot drift from what is accepted.
func reviseSystemPrompt() string {
	props := make([]string, 0, len(TimelineProperties))
	for _, p := range TimelineProperties {
		r := timelineRanges[p]
		props = append(props, fmt.Sprintf("%s (%g to %g)", p, r[0], r[1]))
	}
	return `You change the MOTION of one existing page that will be rendered to a video. You are shown the page (its paint may be left out: you only need its structure, its layout and its timeline) and a request. Answer with ONE JSON object only: the new content of the page's <script type="application/json" id="motion-timeline"> block. No explanations, no markdown fences, no HTML.

RULES
- Shape: {"version":1,"tracks":[{"id":"title","target":".title","start":0.2,"duration":0.9,"ease":"out","stagger":0.12,"from":{"opacity":0,"y":36},"to":{"opacity":1,"y":0}}]}.
- Properties of "from" and "to": ` + strings.Join(props, ", ") + `. Curves for "ease": ` + strings.Join(TimelineCurves, ", ") + ` (or four numbers of a cubic-bezier). x and y are pixels, rotate is degrees.
- Every "target" is a CSS selector that matches an element of the page you are shown: never invent one. Every "id" is unique. "start" is 0 or more and "duration" more than 0, both in seconds; with "stagger", each matched element starts that many seconds after the previous one.
- Every track must end before the clip does: start + (elements - 1) * stagger + duration is at most the duration given.
- An element the timeline animates must not also have a CSS animation.
- Change only what the request asks for and keep every other track exactly as it is.`
}

func reviseUser(instruction, seen string, r Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "REQUEST: %s\n", strings.TrimSpace(instruction))
	fmt.Fprintf(&b, "CLIP: %dx%d, %.2f seconds.\n", r.Width, r.Height, r.Duration)
	if r.Overlay {
		b.WriteString("The page is an OVERLAY drawn over a picture or video.\n")
	}
	b.WriteString("\nPAGE (target only elements that appear in it):\n")
	b.WriteString(seen)
	return b.String()
}

func reviseRepair(user, timeline string, problems []string) string {
	return user + "\n\nYour previous timeline had these problems:\n- " + strings.Join(problems, "\n- ") +
		"\n\nHere is the previous timeline. Answer with the corrected JSON only.\n\n" + timeline
}

// pageCheck is what a revised page goes through, the same check as a new page (see Check).
type pageCheck func(ctx context.Context, p Page) ([]string, error)

// Revise changes the motion of a stored page as the request says and returns the new page. The model answers with the
// timeline block only; the page keeps every other byte. The page needs a timeline block (ErrNoTimeline otherwise, and the
// model is not called). A timeline that does not pass is requested again with the problems, up to MaxAttempts times, and
// then ErrTimelineRejected is returned with the last problems. view may be nil (the model is shown the page itself).
func Revise(ctx context.Context, w Writer, browser, page, instruction string, r Request, lim Limits, view Viewer) (string, error) {
	return revise(ctx, w, page, instruction, r, view, func(ctx context.Context, p Page) ([]string, error) {
		return Check(ctx, browser, p, lim)
	})
}

func revise(ctx context.Context, w Writer, page, instruction string, r Request, view Viewer, check pageCheck) (string, error) {
	if strings.TrimSpace(instruction) == "" {
		return "", errors.New("the request to change the motion is empty")
	}
	if _, ok := ExtractTimeline(page); !ok {
		return "", ErrNoTimeline
	}
	seen := page
	if view != nil {
		// a view that lost the timeline is of no use to the model: show it the page
		if v := view(ctx, page); strings.Contains(v, "motion-timeline") {
			seen = v
		}
	}
	system := reviseSystemPrompt()
	user := reviseUser(instruction, seen, r)
	var last []string
	for attempt := 0; attempt < MaxAttempts; attempt++ {
		answer, err := w.Write(ctx, system, user, reviseMaxTokens)
		if err != nil {
			return "", err
		}
		timeline := ExtractTimelineJSON(answer)
		problems := ValidateTimeline(timeline)
		if len(problems) == 0 {
			timeline = compactJSON(timeline)
			next, _ := SpliceTimeline(page, timeline)
			problems, err = check(ctx, Page{HTML: next, Width: r.Width, Height: r.Height, Duration: r.Duration, Transparent: r.Overlay})
			if err != nil {
				return "", err
			}
			if len(problems) == 0 {
				return next, nil
			}
		}
		last = problems
		user = reviseRepair(reviseUser(instruction, seen, r), timeline, problems)
	}
	return "", fmt.Errorf("%w after %d attempts: %s", ErrTimelineRejected, MaxAttempts, strings.Join(last, "; "))
}
