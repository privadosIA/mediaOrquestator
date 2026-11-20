package videoedit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// samples copies a result into the folder VIDEOEDIT_SAMPLES_DIR names, so a person can look at what the tools make
// on a real clip. It does nothing when the variable is not set.
func samples(t *testing.T, name string, res testResult) {
	t.Helper()
	dir := os.Getenv("VIDEOEDIT_SAMPLES_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ext := filepath.Ext(res.File)
	if b, err := exec.Command("cp", filepath.Join(res.dir, res.File), filepath.Join(dir, name+ext)).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
}

// The whole clip of the person, through each tool, for the owner to look at.
func TestSamples_RemoveBackground(t *testing.T) {
	c := NewCatalog()
	if os.Getenv("VIDEOEDIT_SAMPLES_DIR") == "" {
		t.Skip("VIDEOEDIT_SAMPLES_DIR is not set")
	}
	realVision(t, c)
	src := personClip(t, 15)
	green, _ := runTool(t, c, RemoveBackground, src, map[string]any{"background": "#00ff00"})
	samples(t, "quitar-fondo-verde", green)
	alpha, _ := runTool(t, c, RemoveBackground, src, map[string]any{"alpha": true})
	samples(t, "quitar-fondo-alfa", alpha)
}

func TestSamples_Depth(t *testing.T) {
	c := NewCatalog()
	if os.Getenv("VIDEOEDIT_SAMPLES_DIR") == "" {
		t.Skip("VIDEOEDIT_SAMPLES_DIR is not set")
	}
	realVision(t, c)
	res, _ := runTool(t, c, Depth, personClip(t, 15), nil)
	samples(t, "profundidad", res)
}

// The networks against their fallbacks on real generated video, for the owner to look at, with the time each takes.
func TestSamples_Upscalers(t *testing.T) {
	c := NewCatalog()
	dir := os.Getenv("VIDEOEDIT_SAMPLES_DIR")
	if dir == "" {
		t.Skip("VIDEOEDIT_SAMPLES_DIR is not set")
	}
	realToolsOrSkip(t, c, "realesrgan", "rife")
	h3 := filepath.Join(dir, "extender-h3.mp4")
	cats := filepath.Join(dir, "motion-control", "1-resultado-gatos.mp4")
	for _, f := range []string{h3, cats} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("%s is not there", f)
		}
	}
	timed := func(name string, tool, src string, settings map[string]any) {
		t0 := time.Now()
		res, warned := runTool(t, c, tool, src, settings)
		t.Logf("%s: %.1f s, engine %s, %dx%d at %.0f fps, notes %v %s", name, time.Since(t0).Seconds(), res.Engine, res.Width, res.Height, res.FPS, res.Notes, warned)
		samples(t, name, res)
	}
	timed("escalar-rapido-x2", Scale, h3, map[string]any{"factor": 2, "engine": "fast"})
	timed("escalar-calidad-x2", Scale, h3, map[string]any{"factor": 2, "engine": "quality"})
	timed("fps-red-30", Interpolate, cats, map[string]any{"fps": 30})
	// the same clip with the networks hidden: the filter is what the tool falls back to
	real := c.ToolsDir
	c.ToolsDir = func() string { return t.TempDir() }
	timed("fps-filtro-30", Interpolate, cats, map[string]any{"fps": 30})
	c.ToolsDir = real
}
