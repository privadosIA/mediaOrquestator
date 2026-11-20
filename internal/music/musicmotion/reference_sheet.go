package musicmotion

// reference_sheet.go: what the proposals are told of a reference video the piece names (spec music-motion, "A reference
// steers the proposal and the script"): its rhythm sheet as a paragraph and the median length of its shots in each third.
// The frames, the words and the colours of the reference never travel here.

import (
	"fmt"
	"math"
	"strings"
)

// ReferenceSheet is the rhythm of a reference: the sheet written as text (see motionref.RhythmText) and the median length
// of its shots, in seconds, in each third of the video (0 when a third has none).
type ReferenceSheet struct {
	Text              string
	MedianShotByThird [3]float64
}

// minReferenceBeats and maxReferenceBeats are the shortest and the longest piece, in beats, the fixed rule makes after a
// reference. A reference with a burst of cuts has a median shot under a second in the part of the video that bursts, and a
// scene needs more than that to be written for: two beats is the floor.
const (
	minReferenceBeats = 2
	maxReferenceBeats = 8
)

// designBlock is the line the design proposal reads ("" without a sheet): the pace as a cue, not a model to copy.
func (r *ReferenceSheet) designBlock() string {
	if r == nil || strings.TrimSpace(r.Text) == "" {
		return ""
	}
	return "Rhythm of the reference video the person likes (a cue for the pace and the kind of shots, not something to copy): " + strings.TrimSpace(r.Text) + "\n"
}

// scriptBlock is what the script proposal reads: the paragraph and the median shot of each third.
func (r *ReferenceSheet) scriptBlock() string {
	if r == nil || strings.TrimSpace(r.Text) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("Rhythm of the reference video the person likes (a cue, not something to copy): " + strings.TrimSpace(r.Text) + "\n")
	var parts []string
	for _, m := range r.MedianShotByThird {
		if m > 0 {
			parts = append(parts, fmt.Sprintf("%.1f s", m))
		}
	}
	if len(parts) == 3 {
		fmt.Fprintf(&b, "Median shot of the reference by thirds: %s. Aim the length of your pieces at it in the same thirds of the song.\n", strings.Join(parts, ", "))
	}
	return b.String()
}

// target is the length the fixed rule gives the pieces of a third of the song: the reference's median shot of that third,
// between minReferenceBeats and maxReferenceBeats beats. 0 when the reference says nothing of that third.
func (r *ReferenceSheet) target(third int, beat float64) float64 {
	if r == nil || third < 0 || third > 2 || r.MedianShotByThird[third] <= 0 {
		return 0
	}
	return math.Max(minReferenceBeats*beat, math.Min(maxReferenceBeats*beat, r.MedianShotByThird[third]))
}

// thirdOf is the third of a song of the given length a time falls in.
func thirdOf(t, duration float64) int {
	if duration <= 0 {
		return 0
	}
	k := int(t / (duration / 3))
	if k > 2 {
		k = 2
	}
	if k < 0 {
		k = 0
	}
	return k
}

// ReferenceShot is one shot of a reference video as the scene writer is told of it: where it lies, whether its cut fell on a
// beat and, when the vision model read it, what it saw (the closed vocabularies of motionref.ShotReading; empty when unread).
type ReferenceShot struct {
	Start, End float64
	// BeatCut says whether the cut that starts the shot fell on a beat; nil when it is not known.
	BeatCut *bool
	// Subject, Text, TextSize, Entrance and Camera are the reading; Subject is empty when the shot was not read.
	Subject, Text, TextSize, Entrance, Camera string
}

// ReferenceShots are the shots of a reference video and its length.
type ReferenceShots struct {
	Duration float64
	Shots    []ReferenceShot
}

// archetypeAt is the line the scene writer reads about the reference shot at the same relative place of its video as the
// piece [start, end) has in a song of the given length: how long the shot lasted, whether it cut on the beat and, when it was
// read, what was on screen, the size of the text, how it entered and the camera. "" without shots.
func (r *ReferenceShots) archetypeAt(start, end, songDuration float64) string {
	if r == nil || len(r.Shots) == 0 || r.Duration <= 0 || songDuration <= 0 {
		return ""
	}
	rel := (start + end) / 2 / songDuration
	best, bestD := 0, math.Inf(1)
	for i, s := range r.Shots {
		if d := math.Abs((s.Start+s.End)/2/r.Duration - rel); d < bestD {
			best, bestD = i, d
		}
	}
	s := r.Shots[best]
	tail := fmt.Sprintf("the shot lasted %.1f s", s.End-s.Start)
	if s.BeatCut != nil {
		if *s.BeatCut {
			tail += " and cut on the beat"
		} else {
			tail += " and did not cut on the beat"
		}
	}
	if s.Subject == "" || s.Subject == "unknown" {
		return fmt.Sprintf("In the reference video, at this moment %s.", tail)
	}
	var what string
	switch {
	case s.Subject == "typography" || (s.Text != "none" && s.Text != "" && s.Text != "unknown" && s.TextSize == "huge"):
		kind := map[string]string{"word": "single word", "line": "line of text", "block": "block of text"}[s.Text]
		if kind == "" {
			kind = "typography"
		}
		if s.TextSize != "" && s.TextSize != "unknown" {
			kind = s.TextSize + " " + kind
		}
		what = "a " + kind
	default:
		what = map[string]string{"figure": "a figure", "chart": "a chart", "ui": "a screen interface", "photo": "a photograph", "abstract": "abstract shapes", "empty": "an empty frame"}[s.Subject]
		if what == "" {
			what = s.Subject
		}
	}
	parts := []string{what}
	if e := map[string]string{"letter": "entering letter by letter", "word": "entering word by word", "cut": "arriving on a cut", "fade": "fading in", "slide": "sliding in", "scale": "scaling in"}[s.Entrance]; e != "" {
		parts = append(parts, e)
	}
	if c := map[string]string{"static": "static camera", "zoom": "the camera zooming", "pan": "the camera panning", "shake": "a shaking camera"}[s.Camera]; c != "" {
		parts = append(parts, c)
	}
	return fmt.Sprintf("In the reference video, this moment was %s; %s.", strings.Join(parts, ", "), tail)
}
