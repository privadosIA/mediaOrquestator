package musicmotion

// origin.go: where a part of the user's folder came from (spec music-motion-parts, "A part remembers where it came from").
// A part that was drafted from a key frame of a reference video, or that a learning proposed, keeps the title and the
// address of that video in origin.json beside its manifest. Parts that come with the program, and parts written by hand,
// have none.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// originFile is the name of the file that holds the origin of a part.
const originFile = "origin.json"

// PartOrigin says which reference video a part came from.
type PartOrigin struct {
	// ReferenceID is the reference of the library (it may have been deleted since); Title and Video are its title and the
	// address of the video, kept so that they outlive it.
	ReferenceID string `json:"reference_id,omitempty"`
	Title       string `json:"title,omitempty"`
	Video       string `json:"video,omitempty"`
	// Frame is the key frame the draft was inspired by, when it was.
	Frame string `json:"frame,omitempty"`
	// Learned says that a learning proposed the part (LearningID names it).
	Learned    bool      `json:"learned,omitempty"`
	LearningID string    `json:"learning_id,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
	// Source, Block, Version, Licence and URL say that the part is a block of another project converted by the program
	// (Source "hyperframes": hfimport): the project, the block's name, its version, its licence and its address.
	Source  string `json:"source,omitempty"`
	Block   string `json:"block,omitempty"`
	Version string `json:"version,omitempty"`
	Licence string `json:"licence,omitempty"`
	URL     string `json:"url,omitempty"`
	// Files are the pictures of the block embedded in the part (name, kind, bytes).
	Files []OriginFile `json:"files,omitempty"`
}

// OriginFile is a file of a block that travels inside its part.
type OriginFile struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Bytes int    `json:"bytes"`
}

// OriginOfDraft is what an approved draft remembers: nil when the draft has nothing to do with a reference (a sentence, an
// uploaded picture) and no learning made it.
func OriginOfDraft(d Draft) *PartOrigin {
	frame := ""
	if rest, ok := strings.CutPrefix(d.Source, "reference:"); ok {
		if _, f, ok := strings.Cut(rest, "/"); ok {
			frame = f
		}
	}
	if d.ReferenceID == "" && d.Video == "" && frame == "" && d.Origin != OriginLearned {
		return nil
	}
	return &PartOrigin{ReferenceID: d.ReferenceID, Title: d.VideoTitle, Video: d.Video, Frame: frame,
		Learned: d.Origin == OriginLearned, LearningID: d.LearningID, ApprovedAt: time.Now().UTC()}
}

func writeOrigin(dir string, o PartOrigin) error {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, originFile), b, 0o644)
}

// WriteUserPart writes a part into the user's folder (manifest.json, part.html and, when it has one, origin.json) with the name of its
// manifest. It never replaces a part that is there: the error says so.
func WriteUserPart(userDir string, manifest []byte, fragment string, o *PartOrigin) error {
	var head struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifest, &head); err != nil || !reName.MatchString(head.Name) {
		return errors.New("the manifest has no valid name")
	}
	dir := filepath.Join(userDir, head.Name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%w: the part %q already exists in %s", ErrPartExists, head.Name, userDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fail := func(err error) error { _ = os.RemoveAll(dir); return err }
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0o644); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "part.html"), []byte(fragment), 0o644); err != nil {
		return fail(err)
	}
	if o != nil {
		if err := writeOrigin(dir, *o); err != nil {
			return fail(err)
		}
	}
	return nil
}
