package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func videoEditProject(t *testing.T, yml string, sources ...string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, SourcesSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range sources {
		if err := os.WriteFile(filepath.Join(dir, SourcesSubdir, s), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadBytes([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, cfg.ValidateInDir(linked(dir))
}

func TestVideoEdit_AProjectWithAToolValidates(t *testing.T) {
	cfg, err := videoEditProject(t, `
project: api-vid-x
output: {mode: video_edit}
video_edit:
  tool: scale
  source: origen.mp4
  settings: {factor: 4, engine: quality}
`, "origen.mp4")
	if err != nil {
		t.Fatalf("does not validate: %v", err)
	}
	if cfg.VideoEdit.Tool != "scale" || cfg.Output.Mode != ModeVideoEdit {
		t.Errorf("cfg = %+v", cfg.VideoEdit)
	}
}

func TestVideoEdit_MotionControlTakesAPictureAndAMotionInsteadOfAVideo(t *testing.T) {
	if _, err := videoEditProject(t, `
project: p
output: {mode: video_edit}
video_edit: {tool: motion-control, motion: baile, image: referencia.png, settings: {strength: 0.8}}
`, "referencia.png"); err != nil {
		t.Fatalf("a motion and a picture: %v", err)
	}
}

func TestVideoEdit_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name, yml, want string
		sources         []string
	}{
		{"unknown tool", "output: {mode: video_edit}\nvideo_edit: {tool: teleport, source: origen.mp4}", "is unknown", []string{"origen.mp4"}},
		{"no block", "output: {mode: video_edit}", "needs a video_edit block", nil},
		{"setting out of limits", "output: {mode: video_edit}\nvideo_edit: {tool: scale, source: origen.mp4, settings: {factor: 3}}", "setting factor", []string{"origen.mp4"}},
		{"no source", "output: {mode: video_edit}\nvideo_edit: {tool: depth}", "source is required", nil},
		{"source missing", "output: {mode: video_edit}\nvideo_edit: {tool: depth, source: origen.mp4}", "not found in sources", nil},
		{"source not a video", "output: {mode: video_edit}\nvideo_edit: {tool: depth, source: origen.png}", "must be a .mp4", []string{"origen.png"}},
		{"source and motion", "output: {mode: video_edit}\nvideo_edit: {tool: motion-control, source: origen.mp4, motion: x, image: r.png}", "not both", []string{"origen.mp4", "r.png"}},
		{"motion for a tool that has none", "output: {mode: video_edit}\nvideo_edit: {tool: scale, motion: x}", "does not take a motion", nil},
		{"picture where there is none", "output: {mode: video_edit}\nvideo_edit: {tool: depth, source: origen.mp4, image: r.png}", "does not take a reference picture", []string{"origen.mp4", "r.png"}},
		{"picture missing", "output: {mode: video_edit}\nvideo_edit: {tool: motion-control, source: origen.mp4}", "image is required", []string{"origen.mp4"}},
		{"scenes", "output: {mode: video_edit}\nvideo_edit: {tool: depth, source: origen.mp4}\nscenes:\n  - {id: a, tts_text: hi}", "has no scenes", []string{"origen.mp4"}},
		{"block in another mode", "output: {mode: images}\nvideo_edit: {tool: depth, source: origen.mp4}", "only valid with output.mode", []string{"origen.mp4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := videoEditProject(t, "project: p\n"+tc.yml, tc.sources...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one with %q", err, tc.want)
			}
		})
	}
}

func TestVideoEdit_TheModeIsListedAmongTheValidOnes(t *testing.T) {
	cfg, _ := LoadBytes([]byte("project: p\noutput: {mode: nope}\n"))
	err := cfg.ValidateInDir(linked(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), `"video_edit"`) {
		t.Fatalf("error = %v", err)
	}
}
