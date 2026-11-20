package generation

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

const idAnimateDiff = "video-animatediff-sd15-v3"

func TestCatalogAnimateDiff_IsTheMotionModuleAlone(t *testing.T) {
	e := catalogEntryByID(t, idAnimateDiff)
	if e.Kind != "video" || len(e.Files) != 1 {
		t.Fatalf("kind %q, %d files", e.Kind, len(e.Files))
	}
	f := e.Files[0]
	if f.Name != "mm_sd15_v3.safetensors" || f.Role != "motion_module" {
		t.Errorf("file = %+v", f)
	}
	if e.Quality != "" {
		t.Errorf("it is an alternative engine, not a rung of the quality scale: %q", e.Quality)
	}
	if p := e.Preset; p == nil || p.Steps != 25 || p.CfgScale == nil || *p.CfgScale != 7 || p.Resolution != "512x512" {
		t.Errorf("preset = %+v", p)
	}
	if e.SizeBytes != 836618008 {
		t.Errorf("size = %d", e.SizeBytes)
	}
	// no entry bundles an image checkpoint into models/video/: SD 1.5 stays under Imagen
	for _, c := range catalog {
		if c.Kind != "video" {
			continue
		}
		for _, cf := range c.Files {
			if cf.Name == "v1-5-pruned-emaonly.safetensors" {
				t.Errorf("%s must not bundle the SD 1.5 checkpoint", c.ID)
			}
		}
	}
}

func TestCatalogAnimateDiff_InstalledWhenTheModuleIsInModelsVideo(t *testing.T) {
	s, _ := newProjectsServer(t)
	installed := func() bool {
		rec := call(s.Handler(), http.MethodGet, "/api/assets/catalog", "")
		var out struct {
			Entries []catalogEntry `json:"entries"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, e := range out.Entries {
			if e.ID == idAnimateDiff {
				return e.Installed
			}
		}
		t.Fatal("entry missing")
		return false
	}
	if installed() {
		t.Fatal("not installed before the module exists")
	}
	dir := filepath.Join(s.ProjectsDir, "..", "models", "video")
	if k, ok := findKind("video"); ok {
		dir = s.kindDir(k)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mm_sd15_v3.safetensors"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !installed() {
		t.Error("installed once mm_sd15_v3.safetensors is in models/video/")
	}
}
