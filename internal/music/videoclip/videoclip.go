// Package videoclip plans a music video from a song: it splits the lyrics into sections, gives each section the
// share of the song's length its words deserve, cuts every section into shots of about the same length and
// gives each shot a camera movement and the lines it covers. The pictures are described by the shot prompts,
// which another part writes (an assistant, or FallbackPrompt when none is available).
package videoclip

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Section is a part of the lyrics: the tag it had ([Verse], [Chorus]...) and its lines.
type Section struct {
	Tag   string
	Lines []string
}

var sectionTag = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`)

// ParseLyrics splits lyrics into sections by their [tags]. Lines before the first tag form an untagged section;
// blank lines are dropped. Lyrics with no text give no sections.
func ParseLyrics(lyrics string) []Section {
	var out []Section
	cur := -1
	for _, line := range strings.Split(strings.ReplaceAll(lyrics, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := sectionTag.FindStringSubmatch(line); m != nil {
			out = append(out, Section{Tag: strings.TrimSpace(m[1])})
			cur = len(out) - 1
			continue
		}
		if cur < 0 {
			out = append(out, Section{})
			cur = 0
		}
		out[cur].Lines = append(out[cur].Lines, line)
	}
	return out
}

// Shot is one scene of the video: what it covers of the song and how it moves.
type Shot struct {
	ID      string
	Section string
	// Start and Seconds are where the shot sits in the song and how long it lasts there.
	Start   float64
	Seconds float64
	// Duration is the length of the scene: the crossfade into the next one eats its last seconds, so every
	// scene but the last lasts the transition longer, and the video ends where the song does.
	Duration float64
	Lines    []string
	Motion   string
	// Intensity is how strong the camera movement is, 0 to 1.
	Intensity float64
	Prompt    string
}

// Options are what the plan needs.
type Options struct {
	Lyrics string
	// AudioSeconds is the length of the song.
	AudioSeconds float64
	// ShotSeconds is the wanted length of a shot (4 to 12); the plan raises it when the song would need more
	// than MaxShots.
	ShotSeconds float64
	// TransitionSeconds is the crossfade between scenes.
	TransitionSeconds float64
	// Envelope is the loudness of the song every EnvelopeHop seconds. When given, the cuts between sections are
	// moved to the nearest clear change of loudness instead of staying where the text puts them.
	Envelope []float64
}

// Limits of a plan.
const (
	MinShotSeconds     = 3.0
	DefaultShotSeconds = 6.0
	MaxShots           = 60
	// emptySectionLines is how many lines of text a section with none (an intro, an instrumental break) weighs.
	emptySectionLines = 2.0
)

var motions = []string{"zoom_in", "pan_left", "zoom_out", "pan_right"}

// isChorus says whether a tag is the chorus: its shots move more.
func isChorus(tag string) bool {
	t := strings.ToLower(tag)
	return strings.Contains(t, "chorus") || strings.Contains(t, "hook") || strings.Contains(t, "estribillo") || strings.Contains(t, "coro")
}

// Build splits the song into shots. The time of each section is proportional to the text it sings (an empty
// section weighs a couple of lines); a song with no lyrics is cut evenly.
func Build(o Options) ([]Shot, error) {
	shots, _, err := BuildAligned(o)
	return shots, err
}

// BuildAligned is Build that also says how many cuts between sections it moved to a change of the song's
// loudness (0 without an envelope, or when the song has no clear change near them).
func BuildAligned(o Options) ([]Shot, int, error) {
	if o.AudioSeconds <= 0 {
		return nil, 0, fmt.Errorf("videoclip: the song has no length")
	}
	shotLen := o.ShotSeconds
	if shotLen <= 0 {
		shotLen = DefaultShotSeconds
	}
	if shotLen < MinShotSeconds {
		shotLen = MinShotSeconds
	}
	if o.TransitionSeconds < 0 || o.TransitionSeconds >= shotLen {
		return nil, 0, fmt.Errorf("videoclip: the transition (%vs) must be shorter than a shot (%vs)", o.TransitionSeconds, shotLen)
	}
	sections := ParseLyrics(o.Lyrics)
	if len(sections) == 0 {
		sections = []Section{{Tag: "Instrumental"}}
	}
	// Few enough shots: when the song is long for the shot length the shots grow.
	if n := math.Ceil(o.AudioSeconds / shotLen); n > MaxShots {
		shotLen = o.AudioSeconds / MaxShots
	}
	starts, moved := SectionStarts(sections, o.AudioSeconds, o.Envelope)

	var shots []Shot
	at := 0.0
	for i, s := range sections {
		end := o.AudioSeconds
		if i+1 < len(sections) {
			end = starts[i+1]
		}
		span := end - starts[i]
		n := int(math.Max(1, math.Round(span/shotLen)))
		per := span / float64(n)
		for k := 0; k < n; k++ {
			sh := Shot{Section: s.Tag, Start: at, Seconds: per, Lines: sliceLines(s.Lines, k, n)}
			at += per
			shots = append(shots, sh)
		}
	}
	// The last shot ends exactly where the song does, whatever the rounding.
	shots[len(shots)-1].Seconds += o.AudioSeconds - at
	for i := range shots {
		shots[i].ID = fmt.Sprintf("%02d_%s", i+1, slug(shots[i].Section))
		shots[i].Motion = motions[i%len(motions)]
		shots[i].Intensity = 0.35
		if isChorus(shots[i].Section) {
			shots[i].Intensity = 0.55
		}
		shots[i].Duration = shots[i].Seconds
		if i < len(shots)-1 {
			shots[i].Duration += o.TransitionSeconds
		}
		shots[i].Prompt = FallbackPrompt("", shots[i])
	}
	return shots, moved, nil
}

// sliceLines gives the k-th of n parts of the lines (an even split; a part may be empty when there are fewer lines than parts).
func sliceLines(lines []string, k, n int) []string {
	if len(lines) == 0 {
		return nil
	}
	lo := k * len(lines) / n
	hi := (k + 1) * len(lines) / n
	return lines[lo:hi]
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug is a tag as part of a scene id: "Pre-Chorus" -> "pre-chorus", "" -> "shot".
func slug(tag string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(tag), "-"), "-")
	if s == "" {
		return "shot"
	}
	return s
}

// FallbackPrompt is the picture of a shot when nothing wrote a better one: the style of the song and what is sung.
func FallbackPrompt(style string, s Shot) string {
	var b strings.Builder
	b.WriteString("Cinematic music video still")
	if st := strings.TrimSpace(style); st != "" {
		b.WriteString(", " + st)
	}
	if len(s.Lines) > 0 {
		b.WriteString(", a scene that illustrates: " + strings.Join(s.Lines, " / "))
	}
	b.WriteString(". Beautiful lighting, highly detailed.")
	return b.String()
}

// SectionStarts says where each section starts in a song of the given length: in proportion to the text it
// sings (an empty section weighs a couple of lines), then moved to the nearest clear change of loudness when an
// envelope is given. It also returns how many starts moved. The first start is always 0.
func SectionStarts(sections []Section, total float64, env []float64) ([]float64, int) {
	weights := make([]float64, len(sections))
	var sum float64
	for i, s := range sections {
		w := emptySectionLines
		if len(s.Lines) > 0 {
			w = 0
			for _, l := range s.Lines {
				w += 1 + float64(len([]rune(l)))/40 // a longer line takes longer to sing
			}
		}
		weights[i] = w
		sum += w
	}
	starts := make([]float64, len(sections))
	for i := 1; i < len(sections); i++ {
		starts[i] = starts[i-1] + total*weights[i-1]/sum
	}
	moved := 0
	if len(env) > 0 && len(sections) > 1 {
		var inner []float64
		inner, moved = RefineBoundaries(starts[1:], env, total)
		copy(starts[1:], inner)
	}
	return starts, moved
}
