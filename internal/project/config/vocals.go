package config

import (
	"sort"
	"strings"
)

// vocalPhrases maps each accepted `vocals` value to the English descriptor the
// engine reads in a caption. The engine has no voice parameter: the voice is
// asked for in the caption, so the model may follow it loosely. Only the voices that came out in
// listening tests are offered: chipmunk, elder and robot did not.
var vocalPhrases = map[string]string{
	"female": "female vocals",
	"male":   "male vocals",
	"duet":   "male and female duet vocals",
	"child":  "young child vocals",
	"choir":  "choir vocals",
	"deep":   "deep bass male vocals",
}

// VocalStyles returns the accepted values of `vocals`, sorted.
func VocalStyles() []string {
	out := make([]string, 0, len(vocalPhrases))
	for k := range vocalPhrases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ValidVocals reports whether v is an accepted value of `vocals`.
func ValidVocals(v string) bool {
	_, ok := vocalPhrases[v]
	return ok
}

// ResolvedVocals returns the track's voice, or the project's default when the
// track sets none ("" = left to the model).
func (m *Music) ResolvedVocals(t Track) string {
	if t.Vocals != "" {
		return t.Vocals
	}
	if m != nil {
		return m.Vocals
	}
	return ""
}

// CaptionFor returns the caption of a track: its prompt with the style prefix
// in front and the chosen voice at the end. Instrumental tracks and stem
// extractions have no voice to ask for.
func (m *Music) CaptionFor(t Track) string {
	caption := m.Caption(t.Prompt)
	if t.Instrumental || t.Task == TaskExtract {
		return caption
	}
	if phrase, ok := vocalPhrases[m.ResolvedVocals(t)]; ok {
		return strings.TrimRight(strings.TrimSpace(caption), ",") + ", " + phrase
	}
	return caption
}
