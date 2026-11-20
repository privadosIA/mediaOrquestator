package assist

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// TaskDataset writes a dataset of several entries from one sentence, in the text lines the import reads. Its reply is
// a list of blocks, so it goes through Dataset, not Run.
const TaskDataset Task = "dataset"

// Dataset modes.
const (
	DatasetVideo  = "video"
	DatasetMotion = "motion"
	DatasetMusic  = "music"
)

// MaxDatasetEntries is how many entries one call writes at most.
const MaxDatasetEntries = 12

const datasetTokensPerEntry = 160

// DatasetInput is what the dataset writer is told besides the sentence.
type DatasetInput struct {
	Mode     string
	Text     string
	Language string
	// Engine is the video generator the scenes are for (it shapes the Motion lines); Look the style words of every picture.
	Engine string
	Look   string
	// Parts are the catalog parts a motion dataset may name.
	Parts []PlanPart
}

// DatasetEntry is one block the model wrote: its id (may be empty or repeated), its title and the lines of its text.
type DatasetEntry struct {
	ID, Title, Body string
}

const datasetFormat = "Reply with one block per entry and nothing else (no preamble, no code fence). Each block starts with a line " +
	"`## <id> | <title>` (id: a short lowercase code such as s01, s02...; title: at most 6 words in the language of the request) " +
	"followed by the lines of the entry. "

func datasetSystem(in DatasetInput) string {
	switch in.Mode {
	case DatasetMotion:
		return "You write a dataset of animated graphics (titles, charts, counters, lower thirds) for a video from one request written in any language. " + datasetFormat +
			"The lines of an entry are, in this order: either `Part: <name of one of the parts offered>` (then, when it needs them, `Param <name>: <value>` lines and a `Text: <the words shown>` line) " +
			"or `Graphic: <one sentence in English describing the graphic>` when no offered part fits; then `Duration: <seconds, 2 to 12>`. " +
			"Use only part names and parameter names that are offered. At most 12 entries."
	case DatasetMusic:
		return "You write a list of songs from one request written in any language. " + datasetFormat +
			"The lines of an entry are: one sentence in English describing the song's style, genre, mood and instruments; then `Instrumental: yes` only when it must have no singing. " +
			"Do not write lyrics or tags. At most 12 entries."
	}
	return "You write the storyboard of a video from one request written in any language. " + datasetFormat +
		"The lines of an entry (one scene) are, in this order: `Dialogue: \"<the narration of the scene in the language of the request>\"` (omit the line for a silent scene); " +
		"`Motion: <a short English phrase for the camera or subject movement>`; then ONE line in English of 30 to 45 words describing the picture: subject, setting, light, camera angle and lens, mood, no text on it; this line has NO label before it (not `Picture:`, not `Image:`). " +
		"Keep the same characters and look in every scene. At most 12 entries."
}

func datasetUser(in DatasetInput) string {
	var b strings.Builder
	b.WriteString("Request: " + strings.TrimSpace(in.Text) + "\n")
	if in.Language != "" {
		b.WriteString("Language of the request: " + in.Language + "\n")
	}
	if in.Mode == DatasetVideo {
		if in.Engine != "" {
			b.WriteString("Video generator: " + in.Engine + "\n")
		}
		if in.Look != "" {
			b.WriteString("Look of every picture: " + in.Look + "\n")
		}
	}
	if in.Mode == DatasetMotion && len(in.Parts) > 0 {
		b.WriteString("Parts offered:\n")
		for _, p := range in.Parts {
			d := strings.Join(strings.Fields(p.Description), " ")
			if len(d) > 100 {
				d = d[:100]
			}
			b.WriteString("- " + p.Name + ": " + d + "\n")
		}
	}
	return b.String()
}

// Dataset asks the model for the entries of a dataset with the lifecycle of the other tasks. A refusal is ErrRefused;
// a reply with no readable block is ErrEmpty.
func (a *Assistant) Dataset(ctx context.Context, in DatasetInput) ([]DatasetEntry, error) {
	if err := ValidateText(in.Text); err != nil {
		return nil, err
	}
	switch in.Mode {
	case DatasetVideo, DatasetMotion, DatasetMusic:
	default:
		return nil, fmt.Errorf("%w: mode must be video, motion or music, got %q", ErrInvalidInput, in.Mode)
	}
	reply, _, err := a.generateOnCPU(ctx, datasetSystem(in), datasetUser(in), datasetTokensPerEntry*MaxDatasetEntries)
	if err != nil {
		return nil, err
	}
	if IsRefusal(Clean(reply, false)) {
		return nil, refused(a.Model)
	}
	out := ParseDatasetReply(reply)
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: the model returned no entries", ErrEmpty)
	}
	return out, nil
}

var datasetHeader = regexp.MustCompile(`^[\s*_#]*##\s*([^|\n]*?)\s*(?:\|\s*(.*?))?[\s*_]*$`)

// ParseDatasetReply reads the blocks of TaskDataset. It never fails: a block with no body is dropped, and anything
// before the first header is ignored.
func ParseDatasetReply(reply string) []DatasetEntry {
	s := thinkRe.ReplaceAllString(reply, " ")
	if i := strings.Index(s, "<think>"); i >= 0 {
		s = s[:i]
	}
	var out []DatasetEntry
	var cur *DatasetEntry
	var body []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
		if cur.Body != "" {
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(strings.TrimLeft(line, " \t*_"), "##") {
			if m := datasetHeader.FindStringSubmatch(line); m != nil {
				flush()
				cur = &DatasetEntry{ID: strings.Trim(m[1], "`*_ "), Title: strings.Trim(m[2], "`*_ ")}
				continue
			}
		}
		if cur != nil && !strings.HasPrefix(strings.TrimSpace(line), "```") {
			body = append(body, line)
		}
	}
	flush()
	return out
}
