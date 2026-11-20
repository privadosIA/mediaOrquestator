package musicmotion

import "strings"

// groups.go: a long line of the verse is shown in groups of words, one group at a time, sized by the energy of its section (the caption
// rules of docs/MUSIC_MODE_MOTION.md § Craft). Everything here is worked out from the song map, so two renders draw the same frames.

// Group is a run of the words of a line shown together: the words From to To (exclusive), visible from Open to Close, in the seconds
// of the scene. Outside that time the group is not on the page.
type Group struct {
	From  int     `json:"from"`
	To    int     `json:"to"`
	Open  float64 `json:"open"`
	Close float64 `json:"close"`
}

const (
	// groupLongLine is the most words a line has and is still shown as one line.
	groupLongLine = 6
	// groupOpenBefore is how long before its first word a group opens; groupCloseAfter how long after its last word it is removed.
	groupOpenBefore = 0.2
	groupCloseAfter = 0.12
	// groupPause is the pause between two words that is a good place to break a line.
	groupPause = 0.15
)

// GroupSize is how many words a group has at least and at most in a section of that energy: 2 to 3 where the song is loud, 3 to 5
// in a normal section, 4 to 6 in a calm one.
func GroupSize(energy string) (lo, hi int) {
	switch energy {
	case EnergyLoud:
		return 2, 3
	case EnergyQuiet:
		return 4, 6
	}
	return 3, 5
}

// endsPhrase says whether a word closes a phrase (a comma, a full stop or the like).
func endsPhrase(w string) bool {
	w = strings.TrimSpace(w)
	return w != "" && strings.ContainsAny(w[len(w)-1:], ",.;:!?")
}

// LineGroups splits a line of more than six timed words into groups for a section of the given energy, breaking at punctuation or
// at a pause of 150 ms or more once a group has its least words, and never leaving the last group with a single word. A line of six
// words or fewer, or one whose words have no time, is not grouped (nil).
func LineGroups(l Line, energy string) []Group {
	n := len(l.Words)
	if n <= groupLongLine {
		return nil
	}
	lo, hi := GroupSize(energy)
	var cuts []int // the index of the first word of each group
	for start := 0; start < n; {
		cuts = append(cuts, start)
		end := start
		for ; end < n-1; end++ {
			size := end - start + 1
			if size >= hi {
				break
			}
			if size >= lo && (endsPhrase(l.Words[end].Text) || l.Words[end+1].Start-l.Words[end].End >= groupPause) {
				break
			}
		}
		start = end + 1
	}
	if k := len(cuts); k > 1 && n-cuts[k-1] == 1 { // a last group of one word joins the one before it
		cuts = cuts[:k-1]
	}
	groups := make([]Group, len(cuts))
	for i, from := range cuts {
		to := n
		if i+1 < len(cuts) {
			to = cuts[i+1]
		}
		open := l.Words[from].Start - groupOpenBefore
		if i == 0 {
			open = max(open, l.Start)
		} else {
			open = max(open, l.Words[from-1].End) // never over a word of the group before that is still being sung
		}
		groups[i] = Group{From: from, To: to, Open: round3(max(open, 0)), Close: round3(l.Words[to-1].End + groupCloseAfter)}
	}
	for i := 0; i+1 < len(groups); i++ {
		groups[i].Close = min(groups[i].Close, groups[i+1].Open)
	}
	return groups
}
