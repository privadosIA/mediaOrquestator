package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"mediaOrquestator/internal/assistant/assist"
	"mediaOrquestator/internal/project/config"
	"mediaOrquestator/internal/project/importer"
	"mediaOrquestator/internal/server/api"
	"mediaOrquestator/internal/server/assistant"
)

// DatasetRequest is the body of POST /api/assist/dataset.
type DatasetRequest struct {
	Mode     string `json:"mode"`
	Text     string `json:"text"`
	Language string `json:"language,omitempty"`
	// Engine is the video generator the scenes are for; Negative is written as the shared Negative: line of every scene;
	// Look is a phrase of style added to every picture; Images.Variants adds Variants: to every scene.
	Engine   string `json:"engine,omitempty"`
	Negative string `json:"negative,omitempty"`
	Look     string `json:"look,omitempty"`
	Images   *struct {
		Variants int `json:"variants"`
	} `json:"images,omitempty"`
	// Generator is the music generator whose vocabulary the tracks' tags use.
	Generator string           `json:"generator,omitempty"`
	Examples  []assist.Example `json:"examples,omitempty"`
	Model     string           `json:"model,omitempty"`
	Provider  string           `json:"provider,omitempty"`
}

// DatasetAnswer is the answer of POST /api/assist/dataset: tuples in the shape POST /api/import/preview reads.
type DatasetAnswer struct {
	Entries  [][4]string `json:"entries"`
	Mode     string      `json:"mode"`
	Warnings []string    `json:"warnings"`
}

// maxDatasetTracks bounds the music mode: each track is one more call of the model (its lyrics and tags).
const maxDatasetTracks = 6

var (
	datasetIDRe     = regexp.MustCompile(`[^a-z0-9_-]+`)
	datasetMarkerRe = regexp.MustCompile(`(?i)^(dialogue(\s*\([^)]*\))?|motion|negative|variants|seed|speed|silence|duration|adult|graphic|part|text|bpm|design|param\s+\S+|overlay.*)\s*:`)
	// datasetLabelRe is a label the model puts before the picture line ("Picture: ...", "**Image:** ..."): it is not part of the prompt.
	datasetLabelRe  = regexp.MustCompile(`(?i)^[\s*_]*(picture|image|prompt|scene|shot|visual|description)[\s*_]*:[\s*_]*`)
	datasetSharedRe = regexp.MustCompile(`(?i)^(negative|variants)\s*:`)
	datasetDialogue = regexp.MustCompile(`(?i)^(dialogue(?:\s*\([^)]*\))?)\s*:\s*(.*)$`)
)

// POST /api/assist/dataset {"mode", "text", ...} -> DatasetAnswer
//
// A dataset of several entries from one sentence, written in the lines POST /api/import/preview reads: a storyboard
// (video), animated graphics from the catalog's parts (motion) or songs (music). The model writes the entries; the
// shared lines come from the request; every entry is checked with the import's own mapping before it is returned, and
// one that fails is left out with a warning. It has the guards of the plan: routing and consent, the wait for a run in
// progress, the memory rule, the model's own refusal.
func (h *Handler) handleAssistDataset(w http.ResponseWriter, r *http.Request) {
	var req DatasetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	bad := func(msg string) { api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, msg) }
	switch {
	case req.Mode != assist.DatasetVideo && req.Mode != assist.DatasetMotion && req.Mode != assist.DatasetMusic:
		bad(fmt.Sprintf("mode must be video, motion or music, got %q", req.Mode))
		return
	case req.Language != "" && !api.LanguagePattern.MatchString(req.Language):
		bad("language must be an ISO 639-1 code such as es or en")
		return
	case len(req.Negative) > 500 || len(req.Look) > 200 || len(req.Engine) > 40 || len(req.Generator) > 40:
		bad("negative is limited to 500 characters, look to 200, engine and generator to 40")
		return
	case req.Images != nil && (req.Images.Variants < 1 || req.Images.Variants > 16):
		bad("images.variants must be between 1 and 16")
		return
	}
	if err := assist.ValidateText(req.Text); err != nil {
		bad(err.Error())
		return
	}
	if err := assist.ValidateExamples(req.Examples); err != nil {
		bad(err.Error())
		return
	}
	ctx, ok := h.assistant.Gate(w, r, strings.TrimSpace(req.Provider))
	if !ok {
		return
	}
	ctx = assist.WithExamples(ctx, req.Examples)

	in := assist.DatasetInput{Mode: req.Mode, Text: strings.TrimSpace(req.Text), Language: req.Language,
		Engine: strings.TrimSpace(req.Engine), Look: strings.TrimSpace(req.Look)}
	if req.Mode == assist.DatasetMotion {
		for _, p := range h.mmCatalog().List() {
			in.Parts = append(in.Parts, assist.PlanPart{Name: p.Name, Description: p.Description})
		}
	}
	run := h.assistant.AssistDatasetRun
	if run == nil {
		run = assistant.DefaultAssistDataset
	}
	written, err := run(ctx, in, req.Model)
	if err != nil {
		api.WriteAssistError(w, err)
		return
	}

	ans := DatasetAnswer{Entries: [][4]string{}, Mode: req.Mode, Warnings: []string{}}
	warn := func(format string, a ...any) { ans.Warnings = append(ans.Warnings, fmt.Sprintf(format, a...)) }
	if len(written) > assist.MaxDatasetEntries {
		warn("the model wrote %d entries: only the first %d are kept", len(written), assist.MaxDatasetEntries)
		written = written[:assist.MaxDatasetEntries]
	}
	if req.Mode == assist.DatasetMusic && len(written) > maxDatasetTracks {
		warn("%d tracks were asked: only the first %d are written", len(written), maxDatasetTracks)
		written = written[:maxDatasetTracks]
	}

	seen := map[string]bool{}
	var cand [][4]string
	for i, e := range written {
		id := datasetIDRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(e.ID)), "")
		if len(id) > 24 {
			id = id[:24]
		}
		if id == "" || seen[id] {
			if id != "" {
				warn("entry %d repeated the id %q: it was renamed", i+1, id)
			}
			id = fmt.Sprintf("s%02d", i+1)
			for seen[id] {
				id += "x"
			}
		}
		seen[id] = true
		title := strings.TrimSpace(e.Title)
		if r := []rune(title); len(r) > 80 {
			title = string(r[:80])
		}
		body, ok := h.datasetBody(ctx, &req, e, i, warn)
		if !ok {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		cand = append(cand, [4]string{id, title, "", body})
	}

	// What comes back must preview: the import's own mapping decides which entries are usable.
	raw, _ := json.Marshal(cand)
	entries, rej, err := importer.LoadBytes(raw)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidation, err.Error())
		return
	}
	mapper := importer.Map
	switch req.Mode {
	case assist.DatasetMotion:
		mapper = importer.MapMotion
	case assist.DatasetMusic:
		mapper = importer.MapMusic
	}
	res, err := mapper(entries, rej, importer.Options{Project: "dataset", SilenceSeconds: 2, Parts: h.ws.Parts()})
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidation, err.Error())
		return
	}
	refused := map[string]string{}
	for _, rep := range res.Reports {
		if rep.Rejected != "" {
			refused[rep.ID] = rep.Rejected
		}
	}
	for _, c := range cand {
		if why, no := refused[c[0]]; no {
			warn("entry %q was left out: %s", c[0], why)
			continue
		}
		ans.Entries = append(ans.Entries, c)
	}
	if len(ans.Entries) == 0 {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidation, "the model wrote no usable entry: "+strings.Join(ans.Warnings, "; "))
		return
	}
	api.WriteJSON(w, http.StatusOK, ans)
}

// datasetBody is the text of one entry: what the model wrote plus the lines the request fixes. ok is false when the
// entry could not be completed (a warning says why).
func (h *Handler) datasetBody(ctx context.Context, req *DatasetRequest, e assist.DatasetEntry, i int, warn func(string, ...any)) (string, bool) {
	var lines []string
	for _, l := range strings.Split(e.Body, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	switch req.Mode {
	case assist.DatasetMusic:
		return h.datasetTrack(ctx, req, lines, i, warn)
	case assist.DatasetMotion:
		return strings.Join(lines, "\n"), true
	}
	// video: the request's lines replace any the model wrote; the dialogue is quoted; the look ends the picture line.
	var out []string
	last := -1
	for _, l := range lines {
		if datasetSharedRe.MatchString(l) {
			continue
		}
		if m := datasetDialogue.FindStringSubmatch(l); m != nil {
			v := strings.TrimSpace(m[2])
			if !strings.HasPrefix(v, "\"") {
				v = "\"" + strings.Trim(v, "\"“”") + "\""
			}
			l = m[1] + ": " + v
		}
		if !datasetMarkerRe.MatchString(l) {
			if c := strings.TrimSpace(datasetLabelRe.ReplaceAllString(l, "")); c != "" {
				l = c
			}
			last = len(out)
		}
		out = append(out, l)
	}
	if last < 0 {
		warn("scene %d has no picture line: it was left out", i+1)
		return "", false
	}
	if look := strings.TrimSpace(req.Look); look != "" && !strings.Contains(strings.ToLower(out[last]), strings.ToLower(look)) {
		out[last] = strings.TrimRight(out[last], " .,") + ", " + look
	}
	if n := strings.TrimSpace(req.Negative); n != "" {
		out = append(out, "Negative: "+n)
	}
	if req.Images != nil {
		out = append(out, fmt.Sprintf("Variants: %d", req.Images.Variants))
	}
	return strings.Join(out, "\n"), true
}

// datasetTrack turns the brief of a song into its entry with the music task: caption, lyrics and the generator's tags.
func (h *Handler) datasetTrack(ctx context.Context, req *DatasetRequest, lines []string, i int, warn func(string, ...any)) (string, bool) {
	instrumental := false
	var brief []string
	for _, l := range lines {
		if m := regexp.MustCompile(`(?i)^instrumental\s*:\s*(\w+)`).FindStringSubmatch(l); m != nil {
			instrumental = strings.HasPrefix(strings.ToLower(m[1]), "y")
			continue
		}
		brief = append(brief, l)
	}
	if len(brief) == 0 {
		warn("track %d has no description: it was left out", i+1)
		return "", false
	}
	text := strings.Join(brief, " ")
	musicRun := h.assistant.AssistMusicRun
	if musicRun == nil {
		musicRun = assistant.DefaultAssistMusic
	}
	out, err := musicRun(ctx, text, assist.MusicOptions{Instrumental: instrumental, Engine: req.Generator}, req.Model)
	if err != nil {
		if ctx.Err() == nil {
			warn("track %d could not be written: %s", i+1, err)
		}
		return "", false
	}
	b := []string{strings.TrimSpace(out.Caption)}
	if b[0] == "" {
		b[0] = text
	}
	if instrumental {
		b = append(b, "Lyrics: [Instrumental]")
	} else if l := strings.TrimSpace(out.Lyrics); l != "" {
		b = append(b, "Lyrics:\n"+l)
	}
	if tags := string(config.NormalizeTags(strings.Trim(strings.TrimSpace(out.Tags), ".,"))); tags != "" {
		b = append(b, "Tags: "+tags)
	}
	return strings.Join(b, "\n"), true
}
