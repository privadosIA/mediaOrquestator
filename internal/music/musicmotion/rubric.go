package musicmotion

// rubric.go: what a scene is asked to be, written once and read twice: by the writer as rules (scene_draft.go) and by the
// reviewer as questions with a score each (review.go). Changing a criterion here changes both.

import (
	"fmt"
	"strings"
)

// Criterion is one thing a scene is judged on.
type Criterion struct {
	// Key names it in the reviewer's answer.
	Key string
	// Rule is how the writer is told to meet it; Question is how the reviewer is asked about it.
	Rule     string
	Question string
}

// RubricCriteria are the criteria of a scene, in the order they are printed.
var RubricCriteria = []Criterion{
	{"metaphor", "One visual metaphor for what the verse MEANS (not an illustration of its literal words and not a template of bars, circles or grids), drawn from the world and the device of the video so that it belongs to them.",
		"Is there ONE clear visual metaphor for what the verse means, and not a generic template of bars, circles or grids, and does it belong to the world and the device of the video?"},
	{"lyric", "The lyric is part of the picture: the words appear at the time they are sung, in big display type (the words of the verse at least 4 vh, the word that matters at least 12 vh, nothing under 2 vh), the keyword in the accent. No labels and no captions: the only text of a scene is the verse, its keyword and the concept of the video, never a section name, a time, a counter or a route number (the HUD part draws those).",
		"Is the lyric part of the picture (big type for the words that matter, the keyword in the accent) instead of text pasted over it, readable, and with no label or caption (a section name, a time, a counter) besides it?"},
	{"accent", "One accent colour and one typographic system for the whole scene: the design's own, its accent and its two fonts, and nothing else.",
		"Is there a single accent colour and a single typographic system?"},
	{"motion", "Entrances with ease and overshoot (never linear motion), something changes on every beat, depth from layers that move at different speeds.",
		"Do the frames show depth and a change from one frame to the next, as an animation with ease and not as a static slide?"},
	{"frame", "Everything stays inside the frame with a margin; nothing overlaps its own text.",
		"Is every text completely inside the frame with a margin, with nothing clipped and no text over other text?"},
	{"people", "No people or faces.",
		"Are there no people or faces?"},
}

// Verdict thresholds on the 0 to 10 score.
const (
	PassScore = 7
	WeakScore = 5
)

// Verdicts of a review.
const (
	VerdictPass = "pass"
	VerdictWeak = "weak"
	VerdictFail = "fail"
)

// RubricRules is the rubric as the rules the writer follows, one per line.
func RubricRules() string {
	var b strings.Builder
	for i, c := range RubricCriteria {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(c.Rule)
	}
	return b.String()
}

// MaxCriterionScore is the most the reviewer gives a criterion.
const MaxCriterionScore = 3

// RubricQuestions is the rubric as the questions the reviewer scores from 0 to 3 each.
func RubricQuestions() string {
	var b strings.Builder
	for _, c := range RubricCriteria {
		fmt.Fprintf(&b, "- %s: %s (0 = no, 1 = a little, 2 = mostly, 3 = yes)\n", c.Key, c.Question)
	}
	return b.String()
}

// ScoreOf is the 0 to 10 score of the scores the reviewer gave each criterion (0 to 3; a missing one counts as 0): their sum scaled to 10
// and rounded, so three criteria with 1, 3 and 3 are 7 of 9, which is 8.
func ScoreOf(scores map[string]float64) int {
	total := 0.0
	for _, c := range RubricCriteria {
		v := scores[c.Key]
		if v < 0 {
			v = 0
		}
		if v > MaxCriterionScore {
			v = MaxCriterionScore
		}
		total += v
	}
	return int(total*10/float64(MaxCriterionScore*len(RubricCriteria)) + 0.5)
}

// VerdictOf is the verdict of a score.
func VerdictOf(score int) string {
	switch {
	case score >= PassScore:
		return VerdictPass
	case score >= WeakScore:
		return VerdictWeak
	}
	return VerdictFail
}
