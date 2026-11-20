package config

import (
	"fmt"
	"path/filepath"
)

// Mix is a timeline of clips rendered into one audio file (music mode): the
// clips are cut, faded, scaled, delayed to their start and summed.
type Mix struct {
	ID    string `yaml:"id" json:"id"`
	Clips []Clip `yaml:"clips" json:"clips"`
	// Gain scales the whole mix after the clips are summed, 0 to MaxClipGain
	// (default 1). Normalize then brings it to a standard loudness
	// (NormalizeLUFS), whatever the gains of the clips added up to.
	Gain      *float64 `yaml:"gain,omitempty" json:"gain,omitempty"`
	Normalize bool     `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

// NormalizeLUFS is the integrated loudness a normalized mix is brought to,
// the level music streaming services use.
const NormalizeLUFS = -14

// ResolvedGain returns the mix's master volume factor (default 1).
func (m Mix) ResolvedGain() float64 {
	if m.Gain == nil {
		return 1
	}
	return *m.Gain
}

// Clip is one piece of audio on a mix's timeline. Its audio comes from
// exactly one of Track (a track of the project, Variant 1-based) or File (a
// name in the sources folder). Optional numbers are pointers so "absent"
// differs from an explicit zero.
type Clip struct {
	Track   string `yaml:"track,omitempty" json:"track,omitempty"`
	Variant *int   `yaml:"variant,omitempty" json:"variant,omitempty"`
	File    string `yaml:"file,omitempty" json:"file,omitempty"`
	// Start is the second of the mix where the used part begins (default 0).
	Start *float64 `yaml:"start,omitempty" json:"start,omitempty"`
	// Gain scales the volume, 0 to MaxClipGain (default 1).
	Gain *float64 `yaml:"gain,omitempty" json:"gain,omitempty"`
	Mute bool     `yaml:"mute,omitempty" json:"mute,omitempty"`
	// FadeIn and FadeOut are in seconds, inside the used part.
	FadeIn  *float64 `yaml:"fade_in,omitempty" json:"fade_in,omitempty"`
	FadeOut *float64 `yaml:"fade_out,omitempty" json:"fade_out,omitempty"`
	// TrimStart and TrimEnd select the part of the original to use, in
	// seconds from its start; absent means the whole file.
	TrimStart *float64 `yaml:"trim_start,omitempty" json:"trim_start,omitempty"`
	TrimEnd   *float64 `yaml:"trim_end,omitempty" json:"trim_end,omitempty"`
}

// MaxClipGain is the largest volume factor a clip accepts.
const MaxClipGain = 2.0

func orZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ResolvedStart returns the clip's start in the mix (default 0).
func (c Clip) ResolvedStart() float64 { return orZero(c.Start) }

// ResolvedGain returns the clip's volume factor (default 1).
func (c Clip) ResolvedGain() float64 {
	if c.Gain == nil {
		return 1
	}
	return *c.Gain
}

// ResolvedFadeIn is the fade-in of the clip in seconds (default 0).
func (c Clip) ResolvedFadeIn() float64 { return orZero(c.FadeIn) }

// ResolvedFadeOut is the fade-out of the clip in seconds (default 0).
func (c Clip) ResolvedFadeOut() float64 { return orZero(c.FadeOut) }

// ResolvedVariant returns the 1-based variant of a track clip (default 1).
func (c Clip) ResolvedVariant() int {
	if c.Variant == nil {
		return 1
	}
	return *c.Variant
}

// MixFile returns the exported path of a mix, relative to the project.
func (c *Config) MixFile(id string) string {
	return filepath.Join(c.ExportDir(), id+"."+c.Defaults.Music.ResolvedOutputFormat())
}

// TrackFile returns the exported file of a track variant (1-based) the clip
// refers to, relative to the project, and whether the track and variant exist.
func (c *Config) TrackFile(trackID string, variant int) (string, bool) {
	for _, t := range c.Tracks {
		if t.ID != trackID {
			continue
		}
		n := t.EffectiveVariants()
		if variant < 1 || variant > n {
			return "", false
		}
		return filepath.Join(c.ExportDir(), VariantAudioName(t.ID, n, variant-1, c.Defaults.Music.ResolvedOutputFormat())), true
	}
	return "", false
}

// validateMixes checks the mixes of a music project. Messages name the mix,
// the clip (1-based) and the field.
func (c *Config) validateMixes() []error {
	var errs []error
	trackIDs := make(map[string]bool, len(c.Tracks))
	for _, t := range c.Tracks {
		trackIDs[t.ID] = true
	}
	seen := make(map[string]int, len(c.Mixes))
	for i, m := range c.Mixes {
		if m.ID == "" || !trackIDPattern.MatchString(m.ID) {
			errs = append(errs, fmt.Errorf("config: mix at index %d has an invalid id %q: use letters, digits, '.', '_' or '-'", i, m.ID))
		}
		if prev, dup := seen[m.ID]; dup {
			errs = append(errs, fmt.Errorf("config: duplicate mix id %q (first at index %d, again at index %d)", m.ID, prev, i))
		} else {
			seen[m.ID] = i
		}
		if trackIDs[m.ID] {
			errs = append(errs, fmt.Errorf("config: mix %q has the same id as a track: mixes and tracks share --only and file names, so choose another id", m.ID))
		}
		if len(m.Clips) == 0 {
			errs = append(errs, fmt.Errorf("config: mix %q has no clips", m.ID))
		}
		if m.Gain != nil && (*m.Gain < 0 || *m.Gain > MaxClipGain) {
			errs = append(errs, fmt.Errorf("config: mix %q: gain must be between 0 and %v, got %v", m.ID, MaxClipGain, *m.Gain))
		}
		for j, cl := range m.Clips {
			errs = append(errs, c.validateClip(m.ID, j+1, cl, trackIDs)...)
		}
	}
	return errs
}

func (c *Config) validateClip(mixID string, n int, cl Clip, trackIDs map[string]bool) []error {
	var errs []error
	bad := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf("config: mix %q clip %d: "+format, append([]any{mixID, n}, a...)...))
	}
	switch {
	case cl.Track != "" && cl.File != "":
		bad("track and file cannot both be set: a clip takes its audio from one of them")
	case cl.Track == "" && cl.File == "":
		bad("needs a track (a track of the project) or a file (a name in the sources folder)")
	case cl.Track != "":
		if !trackIDs[cl.Track] {
			bad("track %q is not a track of this project", cl.Track)
		} else if _, ok := c.TrackFile(cl.Track, cl.ResolvedVariant()); !ok {
			bad("track %q has no variant %d", cl.Track, cl.ResolvedVariant())
		}
	default:
		if !ValidSourceName(cl.File) {
			bad("file %q must be the name of an .mp3 or .wav in the sources folder, not a path", cl.File)
		}
	}
	if cl.Variant != nil && cl.Track == "" {
		bad("variant only applies to a clip with a track")
	}
	if cl.Variant != nil && *cl.Variant < 1 {
		bad("variant must be >= 1, got %d", *cl.Variant)
	}
	if cl.Start != nil && *cl.Start < 0 {
		bad("start must be >= 0, got %v", *cl.Start)
	}
	if cl.Gain != nil && (*cl.Gain < 0 || *cl.Gain > MaxClipGain) {
		bad("gain must be between 0 and %v, got %v", MaxClipGain, *cl.Gain)
	}
	if cl.FadeIn != nil && *cl.FadeIn < 0 {
		bad("fade_in must be >= 0, got %v", *cl.FadeIn)
	}
	if cl.FadeOut != nil && *cl.FadeOut < 0 {
		bad("fade_out must be >= 0, got %v", *cl.FadeOut)
	}
	if cl.TrimStart != nil && *cl.TrimStart < 0 {
		bad("trim_start must be >= 0, got %v", *cl.TrimStart)
	}
	if cl.TrimEnd != nil && *cl.TrimEnd <= orZero(cl.TrimStart) {
		bad("trim_end (%v) must be greater than trim_start (%v)", *cl.TrimEnd, orZero(cl.TrimStart))
	}
	if cl.TrimEnd != nil {
		used := *cl.TrimEnd - orZero(cl.TrimStart)
		if used > 0 && cl.ResolvedFadeIn()+cl.ResolvedFadeOut() > used {
			c.warn("mix %q clip %d: the fades (%v s + %v s) are longer than the used part (%v s)", mixID, n, cl.ResolvedFadeIn(), cl.ResolvedFadeOut(), used)
		}
	}
	return errs
}
