// Package videoedit is the catalog of the tools a video can be edited with (spec video-edit-api): their
// identifiers, the settings each accepts and the limits of every setting, the check of a request against them, and
// the registry of the code that runs each tool. It knows nothing of projects, runs or the server: the
// configuration, the pipeline and the HTTP handlers all ask it.
package videoedit

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Tool identifiers, in the stable order the options list them.
const (
	Scale            = "scale"
	Interpolate      = "interpolate"
	Reframe          = "reframe"
	RemoveBackground = "remove-background"
	Depth            = "depth"
	Extend           = "extend"
	MotionControl    = "motion-control"
)

// Kinds of a setting.
const (
	KindChoice  = "choice"
	KindNumber  = "number"
	KindInteger = "integer"
	KindBoolean = "boolean"
	KindString  = "string"
	KindColor   = "color"
)

// MaxPrompt is the longest prompt a tool takes.
const MaxPrompt = 2000

// Setting is one thing a tool lets the client choose.
type Setting struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Values  []any    `json:"values,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Default any      `json:"default,omitempty"`
	// Required says the setting must be given and not blank.
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// Tool is what the options say of one tool; whether it can run now is the server's business.
type Tool struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	NeedsImage    bool      `json:"needs_image"`
	AcceptsMotion bool      `json:"accepts_motion,omitempty"`
	Notes         []string  `json:"notes,omitempty"`
	Settings      []Setting `json:"settings"`
}

func f(v float64) *float64 { return &v }

var tools = []Tool{
	{
		ID: Scale, Name: "Scale", Description: "Makes the picture 2 or 4 times as large in each side, with the same length, frame rate and sound.",
		Settings: []Setting{
			{Name: "factor", Kind: KindChoice, Values: []any{2.0, 4.0}, Default: 2.0, Description: "Times the sides grow; the result cannot pass 8192 pixels in a side."},
			{Name: "engine", Kind: KindChoice, Values: []any{"fast", "quality"}, Default: "fast", Description: "`fast` resamples; `quality` uses a super-resolution network (slower)."},
		},
	},
	{
		ID: Interpolate, Name: "Frame rate", Description: "Raises the frames per second by making the frames in between, with the same length.",
		Settings: []Setting{
			{Name: "fps", Kind: KindChoice, Values: []any{24.0, 30.0, 60.0}, Default: 30.0, Description: "The target; it must be above the video's own rate."},
		},
	},
	{
		ID: Reframe, Name: "Reframe", Description: "Gives the video another aspect ratio, cutting what does not fit (following the person when there is one) or adding bars.",
		Settings: []Setting{
			{Name: "aspect_ratio", Kind: KindChoice, Values: []any{"16:9", "9:16", "1:1", "4:5"}, Default: "16:9"},
			{Name: "mode", Kind: KindChoice, Values: []any{"crop", "pad"}, Default: "crop"},
			{Name: "color", Kind: KindColor, Default: "#000000", Description: "The colour of the bars in `pad` mode."},
		},
	},
	{
		ID: RemoveBackground, Name: "Remove background", Description: "Keeps the people of the video and replaces the rest by a plain colour, or by transparency in a WebM.",
		Settings: []Setting{
			{Name: "background", Kind: KindColor, Default: "#00ff00"},
			{Name: "alpha", Kind: KindBoolean, Default: false, Description: "A WebM with an alpha channel instead of a plain colour."},
		},
		Notes: []string{"It looks for people: a video with nobody in it comes out unchanged, and says so."},
	},
	{
		ID: Depth, Name: "Depth map", Description: "A video of the same size and length whose frames are the depth of the source: near white, far black.",
	},
	{
		ID: Extend, Name: "Extend", Description: "Makes the video longer with a new clip that starts at its last frame.",
		Settings: []Setting{
			{Name: "seconds", Kind: KindInteger, Min: f(2), Max: f(10), Default: 5.0},
			{Name: "prompt", Kind: KindString, Required: true, Description: "What happens next."},
			{Name: "engine", Kind: KindChoice, Values: []any{"minimax-h3", "ltx"}, Description: "The first one available when left out."},
		},
		Notes: []string{"The quality drops with each extension; the result says how many times the video has been extended."},
	},
	{
		ID: MotionControl, Name: "Motion control", Description: "Makes what a picture shows move the way the people of a video do.",
		NeedsImage: true, AcceptsMotion: true,
		Settings: []Setting{
			{Name: "prompt", Kind: KindString, Description: "What the picture shows and does."},
			{Name: "strength", Kind: KindNumber, Min: f(0.2), Max: f(1), Default: 0.6, Description: "How much the motion rules."},
			{Name: "guidance", Kind: KindNumber, Min: f(1), Max: f(10), Default: 6.0, Description: "How closely the prompt is followed (the cfg scale of sd). Lower (about 4) burns the highlights less, but follows the prompt less: on the same pictures it cut the burnt pixels from 36 % to 21 % for a person and from 24 % to 14 % for two cats, and the cats became half human."},
			{Name: "start", Kind: KindNumber, Min: f(0), Default: 0.0, Description: "Seconds into the video (or the motion of the library) where the 33 frames begin."},
		},
		Notes: []string{"For subjects with a human outline.", "33 frames: about 2 seconds.", "Slow (minutes) and soft: the people of the picture do not keep their identity well.",
			"Describe the people and the place as the picture shows them, with natural lighting: words such as «bright» wash the picture out and make it invent."},
	},
}

// Tools lists every tool in the stable order of the options.
func Tools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	return out
}

// Lookup finds a tool by identifier.
func Lookup(id string) (Tool, bool) {
	for _, t := range tools {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

// IDs lists the identifiers in order.
func IDs() []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.ID
	}
	return out
}

// ValidColor says whether s is a colour written as #rrggbb.
func ValidColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func shown(v any) string {
	if n, ok := number(v); ok {
		return fmt.Sprintf("%g", n)
	}
	return fmt.Sprintf("%v", v)
}

// ValidateSettings checks the settings of a tool against its list: no unknown name, every value of its kind and
// inside its limits. It returns the settings with the defaults filled in (numbers as float64), and the first
// problem found, naming the setting.
func ValidateSettings(id string, settings map[string]any) (map[string]any, error) {
	t, ok := Lookup(id)
	if !ok {
		return nil, fmt.Errorf("tool %q is unknown: use one of %s", id, strings.Join(IDs(), ", "))
	}
	known := map[string]Setting{}
	for _, s := range t.Settings {
		known[s.Name] = s
	}
	names := make([]string, 0, len(settings))
	for n := range settings {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if _, ok := known[n]; !ok {
			return nil, fmt.Errorf("setting %q is not one of the tool %s", n, id)
		}
	}
	out := map[string]any{}
	for _, s := range t.Settings {
		v, given := settings[s.Name]
		if s.Required && (!given || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "") {
			return nil, fmt.Errorf("setting %s is required: it must not be blank", s.Name)
		}
		if !given || v == nil {
			if s.Default != nil {
				out[s.Name] = s.Default
			}
			continue
		}
		cv, err := checkSetting(s, v)
		if err != nil {
			return nil, err
		}
		out[s.Name] = cv
	}
	return out, nil
}

func checkSetting(s Setting, v any) (any, error) {
	bad := func(format string, args ...any) (any, error) {
		return nil, fmt.Errorf("setting %s: "+format, append([]any{s.Name}, args...)...)
	}
	switch s.Kind {
	case KindChoice:
		for _, allowed := range s.Values {
			if an, ok := number(allowed); ok {
				if vn, ok := number(v); ok && vn == an {
					return an, nil
				}
			} else if str, ok := v.(string); ok && str == allowed {
				return str, nil
			}
		}
		opts := make([]string, len(s.Values))
		for i, a := range s.Values {
			opts[i] = shown(a)
		}
		return bad("%s is not accepted: use one of %s", shown(v), strings.Join(opts, ", "))
	case KindNumber, KindInteger:
		n, ok := number(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return bad("must be a number")
		}
		if s.Kind == KindInteger && n != math.Trunc(n) {
			return bad("must be a whole number")
		}
		if s.Min != nil && n < *s.Min {
			return bad("%g is below the minimum, %g", n, *s.Min)
		}
		if s.Max != nil && n > *s.Max {
			return bad("%g is above the maximum, %g", n, *s.Max)
		}
		return n, nil
	case KindBoolean:
		b, ok := v.(bool)
		if !ok {
			return bad("must be true or false")
		}
		return b, nil
	case KindString:
		str, ok := v.(string)
		if !ok {
			return bad("must be text")
		}
		if len(str) > MaxPrompt {
			return bad("is longer than %d characters", MaxPrompt)
		}
		return strings.TrimSpace(str), nil
	case KindColor:
		str, ok := v.(string)
		if !ok || !ValidColor(str) {
			return bad("must be a colour written as #rrggbb")
		}
		return strings.ToLower(str), nil
	}
	return bad("has an unknown kind")
}

// Float reads a number setting of a tool; the settings were validated, so a missing one is 0.
func Float(settings map[string]any, name string) float64 {
	n, _ := number(settings[name])
	return n
}

// Int reads a whole-number setting.
func Int(settings map[string]any, name string) int { return int(Float(settings, name)) }

// Str reads a text setting.
func Str(settings map[string]any, name string) string {
	s, _ := settings[name].(string)
	return s
}

// Bool reads a boolean setting.
func Bool(settings map[string]any, name string) bool {
	b, _ := settings[name].(bool)
	return b
}

// --- the code that runs a tool ------------------------------------------------------------------

// Job is what a tool gets to work on. Every path is absolute.
type Job struct {
	// Dir is the project directory and OutDir the folder the result goes in.
	Dir, OutDir string
	// Video and Image are the source video and the reference picture ("" when there is none).
	Video, Image string
	// Motion is the identifier of a motion of the library ("" when the video is the source) and Settings the
	// validated settings of the tool.
	Motion   string
	Settings map[string]any
	// GPU is the card the run was given ("" for the default one).
	GPU string
	// Warn tells the run something that is not a failure (a fallback used, nobody found).
	Warn func(message string)
}

// Result is what a tool made. File is the name of the file inside OutDir.
type Result struct {
	File     string  `json:"file"`
	Format   string  `json:"format"`
	HasAudio bool    `json:"has_audio"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	FPS      float64 `json:"fps,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	// Engine says what made it when there were alternatives (and the fallback when it was used).
	Engine        string   `json:"engine,omitempty"`
	Notes         []string `json:"notes,omitempty"`
	ExtendedTimes int      `json:"extended_times,omitempty"`
}

// Impl is the code behind a tool. Available says whether it can run now and, when not, why.
type Impl struct {
	Run       func(ctx context.Context, job Job) (Result, error)
	Available func() (bool, string)
}
