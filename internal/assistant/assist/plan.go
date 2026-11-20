package assist

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TaskPlan reads one sentence and plans a whole animated video from it: the song's brief and the animation's choices
// (style, level, frame rate, mode, concept) and the parts worth having. Its reply is several KEY=value lines, so it goes
// through Plan, not Run.
const TaskPlan Task = "plan"

// planMaxTokens bounds the reply: a dozen short lines.
const planMaxTokens = 700

// PlanDevices are the storytelling devices of a music video the planner chooses from (the craft rules of docs/MUSIC_MODE_MOTION.md).
var PlanDevices = []string{
	"type carries the argument", "a character does the work", "one object across the song", "life before and after",
	"a number lands", "the result first", "a question the song answers",
}

// PlanDeviceDefault is the device a plan falls to when the model gives none or one that is not offered: what a typographic video is.
const PlanDeviceDefault = "type carries the argument"

// MaxPlanLine is the longest world or hook, in characters.
const MaxPlanLine = 300

// MaxPlanParts is how many parts a plan names at most.
const MaxPlanParts = 6

// PlanPart is a part of the catalog as the planner is shown it.
type PlanPart struct{ Name, Description string }

// PlanInput is what the planner is given besides the sentence: the values it may choose from.
type PlanInput struct {
	Text     string
	Language string
	// Seconds is the length of the song when the caller states it (0: the model reads it from the sentence).
	Seconds    int
	Styles     []string
	Levels     []string
	FrameRates []int
	Modes      []string
	Parts      []PlanPart
}

// PlanPartGuess is a part the model names: Request is the sentence it wrote for a part that is not in the catalog.
type PlanPartGuess struct{ Name, Request string }

// PlanGuess is what the model understood. A field it did not give (or gave in an unreadable form) is empty or zero.
type PlanGuess struct {
	Title        string
	Language     string
	Instrumental bool
	Seconds      int
	Style        string
	Level        string
	FPS          int
	Mode         string
	Concept      string
	// World, Device and Hook are the storyline: where the video happens, what carries it (as the model wrote it: it may not be
	// one of PlanDevices) and what the first two seconds show.
	World  string
	Device string
	Hook   string
	Parts  []PlanPartGuess
}

var planSystem = "You plan an animated music video from one sentence written in any language. " +
	"Reply with these lines and nothing else (no preamble, no code fence), one KEY=value per line:\n" +
	"TITLE=<at most 6 words, in the language of the request>\n" +
	"LANG=<two-letter code of the language of the request>\n" +
	"INSTRUMENTAL=<yes|no> (yes only when the request asks for no singing or no voice)\n" +
	"SECONDS=<length of the song in seconds if the request names one, else 0>\n" +
	"STYLE=<one of the styles offered>\n" +
	"LEVEL=<one of the levels offered>\n" +
	"FPS=<one of the frame rates offered>\n" +
	"MODE=<one of the modes offered>\n" +
	"CONCEPT=<one sentence in English: the visual idea of the video, joining the world, the device and the hook>\n" +
	"WORLD=<one line in English: where the video happens and what it looks like>\n" +
	"DEVICE=<exactly one of: " + strings.Join(PlanDevices, " | ") + ">\n" +
	"HOOK=<one line in English: exactly what the first two seconds show (never a black frame or a logo alone)>\n" +
	"PART=<name> | <one sentence in English saying what it does>  (zero to 6 lines: the parts worth having; prefer the names offered; " +
	"you may invent a new kebab-case name when no offered part does what the video needs)\n" +
	"Choose only values the request names or clearly implies; when it says nothing about a field, pick the most usual offered value."

func planUser(in PlanInput) string {
	var b strings.Builder
	b.WriteString("Request: " + strings.TrimSpace(in.Text) + "\n")
	if in.Language != "" {
		b.WriteString("Language of the request: " + in.Language + "\n")
	}
	if in.Seconds > 0 {
		b.WriteString("Length of the song: " + strconv.Itoa(in.Seconds) + " seconds (given)\n")
	}
	b.WriteString("Styles offered: " + strings.Join(in.Styles, ", ") + "\n")
	b.WriteString("Levels offered: " + strings.Join(in.Levels, ", ") + "\n")
	fps := make([]string, len(in.FrameRates))
	for i, f := range in.FrameRates {
		fps[i] = strconv.Itoa(f)
	}
	b.WriteString("Frame rates offered: " + strings.Join(fps, ", ") + "\n")
	b.WriteString("Modes offered: " + strings.Join(in.Modes, ", ") + "\n")
	b.WriteString("Parts offered:\n")
	for _, p := range in.Parts {
		d := strings.Join(strings.Fields(p.Description), " ")
		if len(d) > 100 {
			d = d[:100]
		}
		b.WriteString("- " + p.Name + ": " + d + "\n")
	}
	return b.String()
}

// Plan asks the model for the plan of a video from one sentence with the same lifecycle as the other tasks (memory rule,
// CPU only, unload). A reply that is a refusal is ErrRefused; one with no readable line is ErrEmpty.
func (a *Assistant) Plan(ctx context.Context, in PlanInput) (PlanGuess, error) {
	if err := ValidateText(in.Text); err != nil {
		return PlanGuess{}, err
	}
	reply, _, err := a.generateOnCPU(ctx, planSystem, planUser(in), planMaxTokens)
	if err != nil {
		return PlanGuess{}, err
	}
	if IsRefusal(Clean(reply, false)) {
		return PlanGuess{}, refused(a.Model)
	}
	g := ParsePlanReply(reply)
	if g.Title == "" && g.Style == "" && g.Concept == "" && len(g.Parts) == 0 {
		return PlanGuess{}, fmt.Errorf("%w: the model returned no plan", ErrEmpty)
	}
	return g, nil
}

var planLine = regexp.MustCompile(`(?im)^[\s*_#>\-]*(TITLE|LANG|INSTRUMENTAL|SECONDS|STYLE|LEVEL|FPS|MODE|CONCEPT|WORLD|DEVICE|HOOK|PART)[\s*_]*[=:][\s*_]*(.*)$`)

// ParsePlanReply reads the lines of TaskPlan. It never fails: what it cannot read is left empty.
func ParsePlanReply(reply string) PlanGuess {
	s := thinkRe.ReplaceAllString(reply, " ")
	if i := strings.Index(s, "<think>"); i >= 0 {
		s = s[:i]
	}
	var g PlanGuess
	for _, m := range planLine.FindAllStringSubmatch(s, -1) {
		v := strings.Trim(strings.TrimSpace(m[2]), "*_`\"' ")
		switch strings.ToUpper(m[1]) {
		case "TITLE":
			g.Title = v
		case "LANG":
			if len(v) >= 2 {
				g.Language = strings.ToLower(v[:2])
			}
		case "INSTRUMENTAL":
			g.Instrumental = strings.HasPrefix(strings.ToLower(v), "y")
		case "SECONDS":
			g.Seconds = leadingInt(v)
		case "STYLE":
			g.Style = strings.ToLower(v)
		case "LEVEL":
			g.Level = strings.ToLower(v)
		case "FPS":
			g.FPS = leadingInt(v)
		case "MODE":
			g.Mode = strings.ToLower(v)
		case "CONCEPT":
			g.Concept = v
		case "WORLD":
			g.World = v
		case "DEVICE":
			g.Device = strings.ToLower(strings.TrimRight(v, ". "))
		case "HOOK":
			g.Hook = v
		case "PART":
			name, req, _ := strings.Cut(v, "|")
			name = strings.ToLower(strings.Trim(strings.TrimSpace(name), "*_`\"'"))
			if name != "" {
				g.Parts = append(g.Parts, PlanPartGuess{Name: name, Request: strings.TrimSpace(req)})
			}
		}
	}
	return g
}

func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}
