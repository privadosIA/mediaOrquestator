package workspace

// blenderwriter.go: the descriptions of 3D scenes (.scene.json in a project's sources): a model writes or revises one from an
// idea, and a person can read and save one by hand. A description is stored only if it passes the same checks as one written
// by hand. See specs/blender-scene-writer.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"mediaOrquestator/internal/platform/runner"
	"mediaOrquestator/internal/project/config"
	"mediaOrquestator/internal/project/pipeline"
	"mediaOrquestator/internal/server/api"
	"mediaOrquestator/internal/video/blender"
	videorender "mediaOrquestator/internal/video/render"
)

// The seconds a description can be written for.
const (
	minDescriptionSeconds = 0.5
	maxDescriptionSeconds = 600
)

// descriptionView is a stored description as the endpoints answer it.
type descriptionView struct {
	Name string `json:"name"`
	// Description is the file as written (what a person edits); Resolved is the same with every key in seconds and every
	// colour #rrggbb, for the project's tempo and design.
	Description json.RawMessage `json:"description"`
	Resolved    *blender.Spec   `json:"resolved,omitempty"`
}

// descriptionProject resolves {id} and the project's settings for a description: the checks' environment, the frame's
// size and the fps. It answers the error itself.
func (h *Handler) descriptionProject(w http.ResponseWriter, r *http.Request) (id string, cfg *config.Config, env blender.Env, ok bool) {
	id, ok = h.RequireProject(w, r)
	if !ok {
		return
	}
	cfg, err := h.ProjectConfig(id)
	if err != nil {
		api.WriteError(w, http.StatusConflict, api.CodeConflict, fmt.Sprintf("the project's configuration cannot be read: %v", err))
		return id, nil, env, false
	}
	return id, cfg, pipeline.BlenderEnv(cfg, h.ProjectDir(id)), true
}

func descriptionName(w http.ResponseWriter, name string) bool {
	if !config.ValidSceneSpecName(name) {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("invalid description name %q: use a plain file name ending in %s", name, config.SceneSpecSuffix))
		return false
	}
	return true
}

func (h *Handler) descriptionPath(id, name string) string {
	return filepath.Join(h.SourcesDir(id), name)
}

// storeDescription writes the description to sources/<name> through a temporary name, so a failure leaves the file that was
// there as it was.
func (h *Handler) storeDescription(id, name string, raw []byte) error {
	dir := h.SourcesDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".description-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), h.descriptionPath(id, name))
}

// indented is a description's JSON as it is stored: indented, with a final line break.
func indented(raw []byte) []byte {
	var b bytes.Buffer
	if err := json.Indent(&b, raw, "", "  "); err != nil {
		return raw
	}
	return append(b.Bytes(), '\n')
}

// POST /api/projects/{id}/blender/descriptions {"name", "idea", "seconds", "revise"?: bool, "provider"?: id}
//
//	-> {name, description, resolved, provider, model, attempts}
//
// Asks the model of the motion_graphics task for the description of a 3D scene (or, with revise, for the stored one changed
// as the idea says) and stores it as sources/<name> only if it passes the checks of a hand-written one.
func (h *Handler) handleDescriptionWrite(w http.ResponseWriter, r *http.Request) {
	id, cfg, env, ok := h.descriptionProject(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     string  `json:"name"`
		Idea     string  `json:"idea"`
		Seconds  float64 `json:"seconds"`
		Revise   bool    `json:"revise"`
		Provider string  `json:"provider"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	req.Idea = strings.TrimSpace(req.Idea)
	switch {
	case !descriptionName(w, req.Name):
		return
	case req.Idea == "":
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, "idea is required: say in words what the scene shows")
		return
	case utf8.RuneCountInString(req.Idea) > blender.MaxIdeaRunes:
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("the idea is longer than %d characters", blender.MaxIdeaRunes))
		return
	case !(req.Seconds >= minDescriptionSeconds && req.Seconds <= maxDescriptionSeconds):
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("seconds must be from %g to %d", minDescriptionSeconds, maxDescriptionSeconds))
		return
	}
	var current []byte
	if req.Revise {
		var err error
		if current, err = os.ReadFile(h.descriptionPath(id, req.Name)); err != nil {
			api.WriteError(w, http.StatusNotFound, api.CodeNotFound, fmt.Sprintf("there is no stored description %q to revise: write it first", req.Name))
			return
		}
	}
	ctx, wr, info, ok := h.motionWriterFor(w, r, req.Provider, "the scene")
	if !ok {
		return
	}
	width, height, _ := videorender.ParseResolution(cfg.Defaults.Resolution) // 0 when unreadable: the prompt says 16:9
	fps := cfg.Defaults.FPS
	wreq := blender.WriteRequest{Idea: req.Idea, Seconds: req.Seconds, FPS: fps, Width: width, Height: height, Env: env,
		Families: pipeline.BlenderFamilies(cfg), Current: current}
	write := h.Overrides.BlenderWrite
	if write == nil {
		write = blender.WriteDescription
	}
	res, err := write(ctx, wr, wreq)
	if err != nil {
		if errors.Is(err, blender.ErrRejected) {
			api.WriteError(w, http.StatusUnprocessableEntity, api.CodeInvalid, err.Error())
			return
		}
		writeMotionError(w, err)
		return
	}
	if err := h.storeDescription(id, req.Name, res.Raw); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternal, fmt.Sprintf("cannot save the description: %v", err))
		return
	}
	api.WriteJSON(w, http.StatusOK, struct {
		descriptionView
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Attempts int    `json:"attempts"`
	}{descriptionView{Name: req.Name, Description: bytes.TrimSpace(res.Raw), Resolved: res.Spec}, info.Provider, info.Model, res.Attempts})
}

// GET /api/projects/{id}/blender/descriptions/{name} -> {name, description, resolved}
func (h *Handler) handleDescriptionGet(w http.ResponseWriter, r *http.Request) {
	id, _, env, ok := h.descriptionProject(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !descriptionName(w, name) {
		return
	}
	raw, err := os.ReadFile(h.descriptionPath(id, name))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, fmt.Sprintf("there is no stored description %q", name))
		return
	}
	view := descriptionView{Name: name, Description: bytes.TrimSpace(raw)}
	if !json.Valid(raw) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeInvalid, fmt.Sprintf("the stored file %q is not valid JSON", name))
		return
	}
	if spec, err := blender.Resolve(raw, env); err == nil { // not checked: a file edited by hand may be wrong, and is shown anyway
		view.Resolved = spec
	}
	api.WriteJSON(w, http.StatusOK, view)
}

// PUT /api/projects/{id}/blender/descriptions/{name} {"description": {...}, "seconds": 5} -> as the GET
//
// Stores a description written by hand after the checks of the writer's (against a scene seconds long); the length is not
// stored: a run checks the description again against the real length of the scene.
func (h *Handler) handleDescriptionPut(w http.ResponseWriter, r *http.Request) {
	id, _, env, ok := h.descriptionProject(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !descriptionName(w, name) {
		return
	}
	var req struct {
		Description json.RawMessage `json:"description"`
		Seconds     float64         `json:"seconds"`
	}
	limit := int64(blender.MaxSpecBytes + 8<<10)
	api.RaiseBodyLimit(w, r, limit)
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	if len(req.Description) == 0 || !(req.Seconds >= minDescriptionSeconds && req.Seconds <= maxDescriptionSeconds) {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("description and seconds (from %g to %d) are required", minDescriptionSeconds, maxDescriptionSeconds))
		return
	}
	spec, err := blender.Parse(req.Description, req.Seconds, env)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, err.Error())
		return
	}
	stored := indented(req.Description)
	if err := h.storeDescription(id, name, stored); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternal, fmt.Sprintf("cannot save the description: %v", err))
		return
	}
	api.WriteJSON(w, http.StatusOK, descriptionView{Name: name, Description: bytes.TrimSpace(stored), Resolved: spec})
}

// POST /api/projects/{id}/blender/descriptions/{name}/preview {"t": 1.5, "seconds": 5, "width"?: 480} -> image/png
//
// Draws one frame of the stored description at second t of a scene seconds long: small, with few samples, on the CPU (it
// never takes a card from a run). Nothing is stored.
func (h *Handler) handleDescriptionPreview(w http.ResponseWriter, r *http.Request) {
	id, cfg, env, ok := h.descriptionProject(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !descriptionName(w, name) {
		return
	}
	var req struct {
		T       float64 `json:"t"`
		Seconds float64 `json:"seconds"`
		Width   int     `json:"width"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	switch {
	case !(req.Seconds >= minDescriptionSeconds && req.Seconds <= maxDescriptionSeconds):
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("seconds must be from %g to %d", minDescriptionSeconds, maxDescriptionSeconds))
		return
	case !(req.T >= 0 && req.T <= req.Seconds):
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("t must be inside the scene (0 to %g s)", req.Seconds))
		return
	case req.Width < 0 || req.Width > blender.MaxPreviewWidth:
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, fmt.Sprintf("width must be at most %d", blender.MaxPreviewWidth))
		return
	}
	spec, err := blender.Load(h.descriptionPath(id, name), req.Seconds, env)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			api.WriteError(w, http.StatusNotFound, api.CodeNotFound, fmt.Sprintf("there is no stored description %q", name))
			return
		}
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalid, err.Error())
		return
	}
	info, err := h.blenderInfo(r)
	if err != nil {
		api.WriteError(w, http.StatusServiceUnavailable, api.CodeUnavailable, err.Error())
		return
	}
	fonts, err := pipeline.BlenderFontPaths(cfg, spec)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternal, err.Error())
		return
	}
	aw, ah, _ := videorender.ParseResolution(cfg.Defaults.Resolution)
	opts := blender.PreviewOptions{T: req.T, Seconds: req.Seconds, FPS: cfg.Defaults.FPS, Width: req.Width, AspectW: aw, AspectH: ah, Sources: h.SourcesDir(id), Fonts: fonts}
	preview := h.Overrides.BlenderPreview
	if preview == nil {
		preview = func(ctx context.Context, bin string, spec *blender.Spec, o blender.PreviewOptions) ([]byte, error) {
			return blender.Preview(ctx, runner.NewExecRunner(), bin, spec, o)
		}
	}
	png, err := preview(r.Context(), info.Path, spec, opts)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternal, fmt.Sprintf("cannot draw the frame: %v", err))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// blenderInfo finds Blender, as configured for tests or on this machine; the error says how to install it.
func (h *Handler) blenderInfo(r *http.Request) (blender.Info, error) {
	if find := h.Overrides.BlenderInfo; find != nil {
		return find(r.Context())
	}
	return blender.Cached(r.Context())
}
