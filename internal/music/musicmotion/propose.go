package musicmotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Writer asks a language model for text. The one thing the proposals need from the providers, so tests replace it.
type Writer interface {
	Write(ctx context.Context, system, user string, maxTokens int) (string, error)
}

// Attempts is how many times the assistant is asked for a valid answer.
const Attempts = 3

// Song is what the assistant is told about the song.
type Song struct {
	Style  string
	Lyrics string
	// SectionTags are the tags of the song's sections, in order.
	SectionTags []string
	// Prompt is the person's direction for the video (at most MaxPrompt characters); empty adds nothing to what the assistant reads.
	Prompt string
	// World and Device are the storyline of the plan the piece was made from (empty for none): the look follows the world and the
	// device, so that every scene belongs to one film.
	World, Device string
	// Reference is the rhythm of a reference video the piece names; nil adds nothing.
	Reference *ReferenceSheet
	// NoAssistant says why there is no writer when the Writer given is nil; it becomes the fallback reason.
	NoAssistant string
}

// MaxPrompt is the longest prompt, in characters.
const MaxPrompt = 2000

// promptBlock is the text the assistant is given for a prompt ("" for none).
func promptBlock(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	return "The person's direction for this video (follow it): " + prompt + "\n"
}

// storylineBlock is what an assistant is told of the plan's storyline: the world and the device of the video, and the hook
// when it is the script that is asked for ("" for none of them).
func storylineBlock(world, device, hook string) string {
	var b strings.Builder
	if w := strings.TrimSpace(world); w != "" {
		b.WriteString("The world of the video (where it happens and what it looks like): " + w + "\n")
	}
	if d := strings.TrimSpace(device); d != "" {
		b.WriteString("The device that carries the video: " + d + "\n")
	}
	if h := strings.TrimSpace(hook); h != "" {
		b.WriteString("The first two seconds show: " + h + "\n")
	}
	return b.String()
}

// fixedRuleNote is added to a reason when a prompt was sent and the fixed rule had to write the part.
func fixedRuleNote(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return " The fixed rule does not read the prompt."
}

const designSystem = `You design the look of a music video made of animated typography and shapes, from a song's style and lyrics.
The design is CHECKED, and a design that breaks a rule is refused:
  1. "dark" and "light" need a contrast of at least 4.5:1 between them.
  2. "accent" needs a contrast of at least 3:1 against the background of EVERY mode the design uses: a section in "dark" mode has "dark" as its background, a section in "light" mode has "light". With both modes in use the accent has to pass against both tones, so it must be clearly lighter than "dark" and clearly darker than "light". If the two tones are too close for that, use ONE mode for every section ("default_mode" and no "modes").
Answer with ONE JSON object and nothing else, with these keys:
  "dark": a hex colour, the dark tone (a background in dark sections, the text in light ones),
  "light": a hex colour, the light tone,
  "accent": a hex colour that stands out against both tones (it paints the keyword of each line),
  "display": the font of the big words, one of: %s,
  "body": the font of the verses, one of the same list,
  "default_mode": "dark" or "light",
  "modes": an object from a section tag in lower case to "dark" or "light" (the chorus usually flips),
  "concept": one sentence (at most 200 characters) naming the visual idea every scene returns to, taken from the lyrics' theme,
  "theme": OPTIONAL, the look every page shares: {"css": a short stylesheet (at most 2000 characters; no @import, no url() except data:, no "<") that sets the type scale and the glow of the words, for example "#verse{font-family:var(--display);font-size:8.4vh;letter-spacing:-.01em;text-transform:uppercase}#verse .k{text-shadow:0 0 .4em var(--accent)}" (only #verse, #hit, :root and classes of your own, never "*" or "body"), "hud": {"tl": at most 40 characters, "tr": ..., "bl": ..., "br": ...}: four tiny corner labels that match the concept (a company name, a ticket number, a place and a date)}.`

func designUser(s Song, problem string) string {
	var b strings.Builder
	b.WriteString(promptBlock(s.Prompt))
	b.WriteString(storylineBlock(s.World, s.Device, ""))
	b.WriteString(s.Reference.designBlock())
	fmt.Fprintf(&b, "Style: %s\nSection tags: %s\nLyrics:\n%s\n", strings.TrimSpace(s.Style), strings.Join(s.SectionTags, ", "), strings.TrimSpace(s.Lyrics))
	if problem != "" {
		fmt.Fprintf(&b, "\nYour previous answer was refused: %s\nAnswer again with the JSON object fixed.\n", problem)
	}
	return b.String()
}

// extractJSON returns the first top-level JSON object found in an answer (models wrap it in text or fences).
func extractJSON(answer string) string {
	start := strings.Index(answer, "{")
	end := strings.LastIndex(answer, "}")
	if start < 0 || end <= start {
		return ""
	}
	return answer[start : end+1]
}

// ProposeDesign asks the assistant for a design and checks it; a refused answer is asked again with the reason, up to
// Attempts times. When none is valid, or the assistant fails, it falls back to a fixed palette with no error. The
// second result says where the design came from ("assistant" or "fallback").
func ProposeDesign(ctx context.Context, w Writer, s Song) (Design, string) {
	d, source, _ := ProposeDesignWhy(ctx, w, s)
	return d, source
}

// ProposeDesignWhy is ProposeDesign that also says why the design is a fallback (empty when the assistant wrote it).
func ProposeDesignWhy(ctx context.Context, w Writer, s Song) (Design, string, string) {
	p := ProposeDesignFull(ctx, w, s)
	return p.Design, p.Source, p.Why
}

// DesignProposal is the answer of ProposeDesignFull.
type DesignProposal struct {
	Design Design
	// Source is "assistant" or "fallback".
	Source string
	// Why says why the design is a fallback; empty when the assistant wrote it.
	Why string
	// Note says what the program changed in the assistant's design (the accent's lightness); empty when nothing.
	Note string
}

// ProposeDesignFull is ProposeDesignWhy that also says what the program did to the design. When the second answer of the assistant is
// refused only because the accent has no contrast, it is not asked a third time: the program moves the accent's lightness
// (AdjustAccent) and keeps the design as the assistant's, with a note.
func ProposeDesignFull(ctx context.Context, w Writer, s Song) DesignProposal {
	why := s.NoAssistant
	if w == nil {
		if why == "" {
			why = "there is no assistant available"
		}
		return DesignProposal{Design: Fallback(s.Style, s.SectionTags), Source: "fallback", Why: why + "." + fixedRuleNote(s.Prompt)}
	}
	problem := ""
	for i := 0; i < Attempts; i++ {
		if ctx.Err() != nil {
			why = "the proposal was cancelled"
			break
		}
		answer, err := w.Write(ctx, fmt.Sprintf(designSystem, strings.Join(FontNames(), ", ")), designUser(s, problem), 1024)
		if err != nil {
			why = "the assistant failed: " + err.Error() // another try would not change it
			break
		}
		d, err := parseDesign(answer)
		if err == nil {
			return DesignProposal{Design: d, Source: "assistant"}
		}
		var contrast *AccentContrastError
		if i == 1 && errors.As(err, &contrast) {
			if fixed, note, ok := settleAccent(d); ok {
				return DesignProposal{Design: fixed, Source: "assistant", Note: note}
			}
		}
		why = "the last answer of the assistant was refused: " + err.Error()
		problem = err.Error() + designHint(d, err)
	}
	return DesignProposal{Design: Fallback(s.Style, s.SectionTags), Source: "fallback", Why: why + "." + fixedRuleNote(s.Prompt)}
}

// settleAccent adjusts the accent of a design that was refused for its contrast and says whether the result is a valid design: the
// accent may not have been the only thing wrong with it (a theme that breaks its rules is left out, as parseDesign does).
func settleAccent(d Design) (Design, string, bool) {
	fixed, note, ok := AdjustAccent(d)
	if !ok {
		return d, "", false
	}
	if fixed.Validate() != nil {
		if fixed.Theme == nil {
			return d, "", false
		}
		fixed.Theme = nil
		if fixed.Validate() != nil {
			return d, "", false
		}
	}
	return fixed, note, true
}

func parseDesign(answer string) (Design, error) {
	raw := extractJSON(answer)
	if raw == "" {
		return Design{}, errors.New("the answer holds no JSON object")
	}
	var d Design
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return Design{}, fmt.Errorf("the answer is not a valid JSON object: %w", err)
	}
	d.Dark, d.Light, d.Accent = strings.TrimSpace(d.Dark), strings.TrimSpace(d.Light), strings.TrimSpace(d.Accent)
	d.Concept = strings.TrimSpace(d.Concept)
	norm := map[string]string{}
	for tag, m := range d.Modes {
		norm[strings.ToLower(strings.TrimSpace(tag))] = m
	}
	d.Modes = norm
	if err := d.Validate(); err != nil {
		// A theme that breaks its rules does not lose a design that is otherwise good: it is left out.
		if d.Theme != nil {
			plain := d
			plain.Theme = nil
			if plain.Validate() == nil {
				return plain, nil
			}
		}
		return d, err // the design as it was written, so that the refusal can be explained
	}
	return d, nil
}

// Resolve gives the design of a piece: the stored one when there is one, and otherwise a new proposal, which it
// stores. A new song therefore never overwrites a design (above all one the user edited); a stored design is
// replaced only when the user asks for a new proposal (force).
func Resolve(ctx context.Context, w Writer, dir string, s Song, force bool) (Stored, error) {
	return ResolveAt(ctx, w, DirPlace(dir), s, force)
}

// ResolveAt is Resolve for a piece kept in any Place.
func ResolveAt(ctx context.Context, w Writer, place Place, s Song, force bool) (Stored, error) {
	if st, ok := LoadDesignAt(place); ok && !force {
		return st, nil
	}
	p := ProposeDesignFull(ctx, w, s)
	st := Stored{Design: p.Design, Source: p.Source, FallbackReason: p.Why, Note: p.Note}
	if err := SaveDesignAt(place, st); err != nil {
		return Stored{}, err
	}
	return st, nil
}

// SetDesign stores a design the user edited after validating it.
func SetDesign(dir string, d Design) (Stored, error) { return SetDesignAt(DirPlace(dir), d) }

// SetDesignAt is SetDesign for a piece kept in any Place.
func SetDesignAt(place Place, d Design) (Stored, error) {
	st := Stored{Design: d, Source: "user", Edited: true}
	if err := SaveDesignAt(place, st); err != nil {
		return Stored{}, err
	}
	return st, nil
}
