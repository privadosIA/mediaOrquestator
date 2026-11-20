package motionref

// rhythm.go: the rhythm sheet of a reference, made from its shots: numbers first, then one deterministic paragraph of text
// written from them for the assistants (the planner, the script and the scene writer). See design D4.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// burstSeconds is the window the burst of cuts is looked for in.
const burstSeconds = 10

// maxRhythmText bounds the paragraph the assistants receive.
const maxRhythmText = 600

// Rhythm is the sheet: how the video is cut and where it bursts and holds.
type Rhythm struct {
	// CutsPerMinute is the number of shot boundaries per minute (hard cuts and content changes) in each third of the video;
	// ChangesPerMinute is how many of them are content changes, and HardCutShare the share of the boundaries that are hard
	// cuts (a typographic video has a low one, a live-action film a high one).
	CutsPerMinute    [3]float64 `json:"cuts_per_minute"`
	ChangesPerMinute [3]float64 `json:"changes_per_minute"`
	HardCutShare     float64    `json:"hard_cut_share"`
	// FirstCut is the time (seconds) of the first cut; absent when the video has none.
	FirstCut *float64 `json:"first_cut,omitempty"`
	// Burst is the window of burstSeconds with most cuts; absent when the video has fewer than two cuts.
	Burst *Burst `json:"burst,omitempty"`
	// Hold is the longest shot of the last two thirds.
	Hold *Hold `json:"hold,omitempty"`
	// LongestShot and MedianShot are in seconds.
	LongestShot float64 `json:"longest_shot"`
	MedianShot  float64 `json:"median_shot"`
	// MedianShotByThird is the median shot length in each third (a shot belongs to the third its start falls in).
	MedianShotByThird [3]float64 `json:"median_shot_by_third"`
	// BeatCutShare is the share of the cuts that fall on a beat, among those whose beat is known; absent without a tempo.
	BeatCutShare *float64 `json:"beat_cut_share,omitempty"`
	// BeatPhase is how far the grid the beat share is measured against sits from the tracked one, in beats: 0 (absent), 0.25,
	// 0.5 or 0.75. It is the phase that puts most boundaries on the beat.
	BeatPhase float64 `json:"beat_phase,omitempty"`
	// StaticShare is the share of the shots that barely move.
	StaticShare float64 `json:"static_share"`
	// TextLedShare, Entrances and Subjects exist only when the shots were read: the share of the read shots made of text
	// filling the frame, and how many of them entered each way and showed each subject.
	TextLedShare *float64       `json:"text_led_share,omitempty"`
	Entrances    map[string]int `json:"entrances,omitempty"`
	Subjects     map[string]int `json:"subjects,omitempty"`
}

// Burst is the busiest window.
type Burst struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Cuts  int     `json:"cuts"`
}

// Hold is the longest shot after the first third.
type Hold struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// buildRhythm makes the sheet of a list of shots of a video of the given length. It carries the readings into the sheet
// when the shots have them.
func buildRhythm(shots []Shot, duration float64) *Rhythm {
	r := &Rhythm{}
	if len(shots) == 0 || duration <= 0 {
		return r
	}
	var cuts []float64
	changes := 0
	for _, s := range shots[1:] {
		cuts = append(cuts, s.Start)
		if s.Kind == KindChange {
			changes++
		}
	}
	if len(cuts) > 0 {
		r.HardCutShare = round2(float64(len(cuts)-changes) / float64(len(cuts)))
	}
	third := duration / 3
	for _, s := range shots[1:] {
		k := int(s.Start / third)
		if k > 2 {
			k = 2
		}
		r.CutsPerMinute[k]++
		if s.Kind == KindChange {
			r.ChangesPerMinute[k]++
		}
	}
	for k := range r.CutsPerMinute {
		r.CutsPerMinute[k] = round1(r.CutsPerMinute[k] / (third / 60))
		r.ChangesPerMinute[k] = round1(r.ChangesPerMinute[k] / (third / 60))
	}
	if len(cuts) > 0 {
		v := round1(cuts[0])
		r.FirstCut = &v
	}
	// The burst: the window of burstSeconds that starts at a cut and holds the most cuts (the first of the ties).
	if len(cuts) >= 2 {
		best, at := 0, 0
		for i, c := range cuts {
			n := 0
			for _, d := range cuts[i:] {
				if d < c+burstSeconds {
					n++
				}
			}
			if n > best {
				best, at = n, i
			}
		}
		start := cuts[at]
		r.Burst = &Burst{Start: round1(start), End: round1(math.Min(start+burstSeconds, duration)), Cuts: best}
	}
	lengths := make([]float64, len(shots))
	byThird := [3][]float64{}
	for i, s := range shots {
		l := s.Length()
		lengths[i] = l
		k := int(s.Start / third)
		if k > 2 {
			k = 2
		}
		byThird[k] = append(byThird[k], l)
		if l > r.LongestShot {
			r.LongestShot = l
		}
	}
	r.LongestShot = round1(r.LongestShot)
	r.MedianShot = round1(median(lengths))
	for k := range byThird {
		r.MedianShotByThird[k] = round1(median(byThird[k]))
	}
	var hold *Shot
	for i := range shots {
		if shots[i].Start < third {
			continue
		}
		if hold == nil || shots[i].Length() > hold.Length() {
			hold = &shots[i]
		}
	}
	if hold != nil {
		r.Hold = &Hold{Start: round1(hold.Start), End: round1(hold.End)}
	}
	// The beat: among the cuts whose beat is known.
	known, on := 0, 0
	for _, s := range shots[1:] {
		if s.BeatCut != nil {
			known++
			if *s.BeatCut {
				on++
			}
		}
	}
	if known > 0 {
		v := round2(float64(on) / float64(known))
		r.BeatCutShare = &v
	}
	static := 0
	for _, s := range shots {
		if s.Static() {
			static++
		}
	}
	r.StaticShare = round2(float64(static) / float64(len(shots)))
	r.addReadings(shots)
	return r
}

// addReadings adds what the vision model saw: the share of text-led shots and the counts of entrances and subjects.
func (r *Rhythm) addReadings(shots []Shot) {
	read, led := 0, 0
	entrances, subjects := map[string]int{}, map[string]int{}
	for _, s := range shots {
		if s.Read.Unknown() {
			continue
		}
		read++
		if s.Read.TextLed() {
			led++
		}
		subjects[s.Read.Subject]++
		if s.Read.Text != "none" && s.Read.Entrance != "" && s.Read.Entrance != "unknown" {
			entrances[s.Read.Entrance]++
		}
	}
	if read == 0 {
		return
	}
	v := round2(float64(led) / float64(read))
	r.TextLedShare = &v
	if len(entrances) > 0 {
		r.Entrances = entrances
	}
	r.Subjects = subjects
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// clock writes seconds as m:ss.
func clock(t float64) string {
	s := int(math.Round(t))
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// RhythmText writes the sheet as one short paragraph, in English, the same text for the same sheet, leaving out what is
// not known.
func RhythmText(r *Rhythm) string {
	if r == nil {
		return ""
	}
	var parts []string
	// With content changes among the boundaries the text says so and how much of the rhythm they are.
	if r.hasChanges() {
		parts = append(parts, fmt.Sprintf("Cuts and text changes: %s/min by thirds (%d %% text changes)",
			join(r.CutsPerMinute[:], func(v float64) string { return trim(v) }), percent(1-r.HardCutShare)))
	} else {
		parts = append(parts, fmt.Sprintf("Cuts: %s/min by thirds", join(r.CutsPerMinute[:], func(v float64) string { return trim(v) })))
	}
	if r.FirstCut != nil {
		parts = append(parts, fmt.Sprintf("first cut at %s s", trim(*r.FirstCut)))
	}
	if r.Burst != nil {
		parts = append(parts, fmt.Sprintf("burst at %s–%s (%d cuts)", clock(r.Burst.Start), clock(r.Burst.End), r.Burst.Cuts))
	}
	if r.Hold != nil {
		parts = append(parts, fmt.Sprintf("longest hold %s s at %s", trim(r.Hold.End-r.Hold.Start), clock(r.Hold.Start)))
	}
	parts = append(parts, fmt.Sprintf("median shot %s s", trim(r.MedianShot)))
	if r.BeatCutShare != nil {
		s := fmt.Sprintf("%d %% of cuts on the beat", percent(*r.BeatCutShare))
		if name := phaseName(r.BeatPhase); name != "" {
			s += " (" + name + " off the tracked grid)"
		}
		parts = append(parts, s)
	}
	parts = append(parts, fmt.Sprintf("%d %% of shots static", percent(r.StaticShare)))
	if r.TextLedShare != nil {
		s := fmt.Sprintf("%d %% of shots are text filling the frame", percent(*r.TextLedShare))
		if e := top(r.Entrances, 3); e != "" {
			s += ", entering by " + e
		}
		parts = append(parts, s)
	}
	if r.Subjects != nil {
		parts = append(parts, "subjects: "+topCounts(r.Subjects, 4))
	}
	text := strings.Join(parts, "; ") + "."
	if len([]rune(text)) > maxRhythmText {
		text = string([]rune(text)[:maxRhythmText-1]) + "…"
	}
	return text
}

// hasChanges says whether any boundary of the sheet is a content change.
func (r *Rhythm) hasChanges() bool {
	for _, c := range r.ChangesPerMinute {
		if c > 0 {
			return true
		}
	}
	return false
}

// phaseName says a beat phase in words; empty for the tracked grid.
func phaseName(phase float64) string {
	switch phase {
	case 0.25:
		return "a quarter of a beat"
	case 0.5:
		return "half a beat"
	case 0.75:
		return "three quarters of a beat"
	}
	return ""
}

func percent(v float64) int { return int(math.Round(v * 100)) }

func trim(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func join(v []float64, f func(float64) string) string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = f(x)
	}
	return strings.Join(out, ", ")
}

// ranked lists the keys of a count map, the most common first, ties by name.
func ranked(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// top writes the n most common entrances as "word (31) or letter (12)".
func top(m map[string]int, n int) string {
	keys := ranked(m)
	if len(keys) > n {
		keys = keys[:n]
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("%s (%d)", k, m[k])
	}
	return strings.Join(out, " or ")
}

// topCounts writes the n most common keys as "typography 38, figure 9".
func topCounts(m map[string]int, n int) string {
	keys := ranked(m)
	if len(keys) > n {
		keys = keys[:n]
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("%s %d", k, m[k])
	}
	return strings.Join(out, ", ")
}
