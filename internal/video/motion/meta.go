package motion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Meta is how the stored page of a scene came to be and what last drew it. It is kept next to
// the page so the clip's details can say that it was a motion graphic, with which browser, and
// which model wrote the page.
type Meta struct {
	// Kind is "motion_graphic" or "overlay".
	Kind string `json:"kind"`
	// Browser is the Chrome or Chromium version that last drew the page.
	Browser string `json:"browser,omitempty"`
	// Provider and Model wrote the page; empty when the page was supplied by the user.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// GeneratedSHA256 is the hash of the page as the model wrote it.
	GeneratedSHA256 string    `json:"generated_sha256,omitempty"`
	GeneratedAt     time.Time `json:"generated_at,omitempty"`
}

// Origins of a page.
const (
	OriginGenerated = "generated" // as the model wrote it
	OriginEdited    = "edited"    // the model wrote it and the user changed it
	OriginSupplied  = "supplied"  // written by the user
)

// MetaPath is where the metadata of a scene's page is kept.
func MetaPath(projectDir, sceneID string, overlay bool) string {
	name := sceneID + ".meta.json"
	if overlay {
		name = sceneID + ".overlay.meta.json"
	}
	return filepath.Join(projectDir, Dir, name)
}

// LoadMeta reads the metadata of a scene's page; ok is false when there is none.
func LoadMeta(projectDir, sceneID string, overlay bool) (Meta, bool) {
	data, err := os.ReadFile(MetaPath(projectDir, sceneID, overlay))
	if err != nil {
		return Meta{}, false
	}
	var m Meta
	if json.Unmarshal(data, &m) != nil {
		return Meta{}, false
	}
	return m, true
}

// StoreMeta writes the metadata of a scene's page.
func StoreMeta(projectDir, sceneID string, overlay bool, m Meta) error {
	path := MetaPath(projectDir, sceneID, overlay)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SHA256 is the hash of a page's text.
func SHA256(html string) string {
	sum := sha256.Sum256([]byte(html))
	return hex.EncodeToString(sum[:])
}

// Origin says whether the page is as the model wrote it, was edited since, or was supplied.
func (m Meta) Origin(currentHTML string) string {
	switch {
	case m.GeneratedSHA256 == "":
		return OriginSupplied
	case m.GeneratedSHA256 == SHA256(currentHTML):
		return OriginGenerated
	default:
		return OriginEdited
	}
}

// Describe is a one-line summary of the metadata for a clip's details.
func (m Meta) Describe(currentHTML string) string {
	kind := "motion graphic"
	if m.Kind == "overlay" {
		kind = "overlay"
	}
	parts := []string{kind}
	switch m.Origin(currentHTML) {
	case OriginGenerated:
		if m.Model != "" {
			parts = append(parts, "page written by "+m.Model)
		}
	case OriginEdited:
		parts = append(parts, "page written by "+m.Model+", then edited")
	case OriginSupplied:
		parts = append(parts, "page written by hand")
	}
	if m.Browser != "" {
		parts = append(parts, "Chrome "+m.Browser)
	}
	return strings.Join(parts, " · ")
}
