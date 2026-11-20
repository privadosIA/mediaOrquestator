package musicmotion

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// The request to the assistant for a draft of a part (spec motion-part-drafts) and the reading of its answer.

// MaxDraftRequest is the longest sentence a draft is asked for with.
const MaxDraftRequest = 500

// DraftInput is what the assistant writes a part from.
type DraftInput struct {
	// Request is what the part has to do (the user's sentence).
	Request string
	// Description is the look of the picture the user gave, written by the vision model; empty when there is none.
	Description string
	// Cat is the catalog the new part has to be different from.
	Cat Catalog
	// BaseManifest and BaseFragment, when given, are a part the assistant changes according to Request instead of writing
	// one from nothing; the name of the base (BaseName) may be kept even if the catalog has it.
	BaseManifest, BaseFragment, BaseName string
}

// draftExample is a small part of the program shown to the model as the technique expected (never to copy).
func draftExample() string {
	manifest, err1 := fs.ReadFile(BuiltinParts(), "counter/manifest.json")
	fragment, err2 := fs.ReadFile(BuiltinParts(), "counter/part.html")
	if err1 != nil || err2 != nil {
		return ""
	}
	return "EXAMPLE of a part that follows all the rules (take the technique, not its content):\n```json\n" + strings.TrimSpace(string(manifest)) +
		"\n```\n```html\n" + strings.TrimSpace(string(fragment)) + "\n```\n"
}

var draftSystem = `You write ONE new part for a library of animated parts that follow a song. A part is two files, which you answer as two fenced blocks and nothing else: first a ` + "```json" + ` block with manifest.json, then a ` + "```html" + ` block with part.html.

manifest.json: {"name": "lower-case-with-dashes (2 to 32 characters, NOT one of the existing names below)", "description": "what it shows and how it moves", "version": 1, "cost_ms": 60, "levels": ["simple","medium","advanced"], "params": [ {"name": "label", "type": "string", "default": "hello", "max_length": 20}, {"name": "count", "type": "int", "default": 3, "min": 1, "max": 6} ]}.
Parameter types are string, int, number, bool and enum (an enum has "options"); EVERY parameter has a default; names are lower case letters, digits and underscores. Use "levels" of only ["simple"] for a plain part, add "medium" and "advanced" if it is richer.

part.html is a FRAGMENT (no <html>, no <body>): HTML, CSS in a <style> and one <script>. Rules:
- It must MOVE: define window.__partSeek = function (t, beat) {...}, called before each frame with the time in seconds and the beat clock beat = {number, position, section, sectionIndex, bpm, time}: number is the index of the last beat (-1 before the first), position how far inside it the frame is (0 up to 1). The beat also carries beat.word and beat.line: the written word being sung ({text, index, line, start, end, position}, position from 0 at its start to 1 at its end, in the seconds of this scene) and its line ({index, text, start, end, position}), each null when nothing is sung (a break between lines); use them to move on the word itself. Draw ONLY from t and beat, so the frames land on the music. No Date, no performance.now, no requestAnimationFrame, no setTimeout, no Math.random (use a small seeded generator if you need chance). No CSS transitions or CSS animations either (they run on the wall clock): set every style directly from t and beat inside __partSeek.
- It must change over every beat (a pulse, a step, a flip on beat.number), not stay still.
- Read the parameters from window.__params. Do not read the song's lyrics: the part draws on its own.
- Colours and fonts come from CSS variables only: var(--bg), var(--fg), var(--accent), var(--display), var(--body). Never write a colour of your own except as a mix of those (color-mix is fine).
- Everything is inline. NEVER load anything from outside (no http(s) URLs, no external images, fonts, scripts or @import) and never write window.__seek.
- The frame is 1280x720 (use vh and vw units). Keep every text fully inside the frame with a margin and do not stack texts over each other. The part is drawn over the song's background and the verse is shown by the page, so leave the lower fifth free.
- Do not draw people or faces.
` + timelineInstruction("part")

// DraftSystem is the instruction of the assistant that writes a part.
func DraftSystem() string { return draftSystem }

// DraftUser is the request: what to do, the look of the picture, the names already taken, an example and, in a later
// attempt, what was wrong with the last answer.
func DraftUser(in DraftInput, problem string) string {
	var b strings.Builder
	if in.BaseManifest != "" || in.BaseFragment != "" {
		b.WriteString("CHANGE this existing part according to the instruction, and answer the whole new version (the two blocks). Keep what the instruction does not touch, and keep its name \"" + in.BaseName + "\" unless the instruction asks for another.\n\nThe part now:\n```json\n" +
			strings.TrimSpace(in.BaseManifest) + "\n```\n```html\n" + strings.TrimSpace(in.BaseFragment) + "\n```\n\nThe instruction:\n" + strings.TrimSpace(in.Request) + "\n")
	} else {
		b.WriteString("What the part has to do:\n" + strings.TrimSpace(in.Request) + "\n")
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		b.WriteString("\nThe look of the picture the user gave as inspiration (take its colours' roles, its shapes and its kind of movement; do not copy text from it):\n" + d + "\n")
	} else {
		b.WriteString("\nThere is no picture: the part is described by that sentence alone.\n")
	}
	var names []string
	for _, p := range in.Cat.ListAll() {
		if p.Name != in.BaseName {
			names = append(names, p.Name)
		}
	}
	b.WriteString("\nNames already taken (yours must be different): " + strings.Join(names, ", ") + "\n\n")
	b.WriteString(draftExample())
	if problem != "" {
		fmt.Fprintf(&b, "\nYour previous answer was refused: %s\nAnswer again with the two blocks fixed.\n", problem)
	}
	return b.String()
}

var (
	reJSONBlock = regexp.MustCompile("(?s)```(?:json|JSON)[^\\n]*\\n(.*?)```")
	reHTMLBlock = regexp.MustCompile("(?s)```(?:html|HTML)[^\\n]*\\n(.*?)```")
)

// ParseDraftAnswer reads the answer of the assistant: the first json block is the manifest and the first html block
// the fragment. The part is then read with the rules of the catalog and its name must be a new one. The problems are
// worded to be given back to the assistant.
func ParseDraftAnswer(answer string, cat Catalog) (manifest []byte, fragment string, part Part, problems []string) {
	return parseDraftAnswer(answer, cat, "")
}

// parseDraftAnswer is ParseDraftAnswer where the name allow may be one the catalog already has (a part being changed).
func parseDraftAnswer(answer string, cat Catalog, allow string) (manifest []byte, fragment string, part Part, problems []string) {
	j := reJSONBlock.FindStringSubmatch(answer)
	h := reHTMLBlock.FindStringSubmatch(answer)
	if j == nil {
		problems = append(problems, "the answer has no ```json block with manifest.json")
	}
	if h == nil {
		problems = append(problems, "the answer has no ```html block with part.html")
	}
	if len(problems) > 0 {
		return nil, "", Part{}, problems
	}
	manifest, fragment = []byte(strings.TrimSpace(j[1])), strings.TrimSpace(h[1])
	part, problems = ParsePart(manifest, fragment)
	if len(problems) > 0 {
		return manifest, fragment, Part{}, problems
	}
	if _, taken := cat.Get(part.Name); taken && part.Name != allow {
		return manifest, fragment, Part{}, []string{fmt.Sprintf("the name %q is already a part of the catalog: choose another name", part.Name)}
	}
	return manifest, fragment, part, nil
}
