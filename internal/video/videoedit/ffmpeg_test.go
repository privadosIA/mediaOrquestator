package videoedit

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestScaleFilter(t *testing.T) {
	f, w, h := scaleFilter(Info{Width: 832, Height: 480}, 2)
	if f != "scale=1664:960:flags=lanczos" || w != 1664 || h != 960 {
		t.Errorf("%s %d %d", f, w, h)
	}
	if _, w, h := scaleFilter(Info{Width: 101, Height: 55}, 4); w%2 != 0 || h%2 != 0 {
		t.Errorf("odd source gave odd sides: %dx%d", w, h)
	}
}

func TestCropWindow_KeepsTheHeightOrTheWidthWhenThatIsSmaller(t *testing.T) {
	for _, tc := range []struct {
		w, h       int
		ratio      float64
		cw, ch     int
		name       string
		ratioLabel string
	}{
		{1920, 1080, 9.0 / 16, 608, 1080, "16:9 to 9:16", "9:16"},
		{1920, 1080, 1, 1080, 1080, "16:9 to 1:1", "1:1"},
		{1920, 1080, 4.0 / 5, 864, 1080, "16:9 to 4:5", "4:5"},
		{1080, 1920, 16.0 / 9, 1080, 608, "9:16 to 16:9", "16:9"},
		{832, 480, 16.0 / 9, 832, 468, "16:9 odd rounding", "16:9"},
	} {
		cw, ch := cropWindow(tc.w, tc.h, tc.ratio)
		if cw != tc.cw || ch != tc.ch || cw > tc.w || ch > tc.h || cw%2 != 0 || ch%2 != 0 {
			t.Errorf("%s: window %dx%d, want %dx%d", tc.name, cw, ch, tc.cw, tc.ch)
		}
	}
}

func TestPadCanvas_HoldsTheWholePicture(t *testing.T) {
	for _, tc := range []struct {
		w, h   int
		ratio  float64
		cw, ch int
	}{
		{1920, 1080, 1, 1920, 1920},
		{1920, 1080, 9.0 / 16, 1920, 3414},
		{1080, 1920, 16.0 / 9, 3414, 1920},
		{1920, 1080, 16.0 / 9, 1920, 1080},
	} {
		cw, ch := padCanvas(tc.w, tc.h, tc.ratio)
		if cw != tc.cw || ch != tc.ch || cw < tc.w || ch < tc.h {
			t.Errorf("%dx%d at %g: canvas %dx%d, want %dx%d", tc.w, tc.h, tc.ratio, cw, ch, tc.cw, tc.ch)
		}
	}
}

func TestReframeFilter(t *testing.T) {
	f, _, _ := reframeFilter(Info{Width: 1920, Height: 1080}, 1, "pad", "#112233")
	if f != "pad=1920:1920:(ow-iw)/2:(oh-ih)/2:color=0x112233" {
		t.Errorf("pad = %s", f)
	}
	f, _, _ = reframeFilter(Info{Width: 1920, Height: 1080}, 1, "crop", "#000000")
	if f != "crop=1080:1080:(iw-1080)/2:(ih-1080)/2" {
		t.Errorf("crop = %s", f)
	}
}

func TestFfmpegArgs(t *testing.T) {
	a := strings.Join(ffmpegArgs("in.mp4", "scale=2:2", "out.mp4", true), " ")
	for _, want := range []string{"-i in.mp4", "-vf scale=2:2", "-map 0:a:0?", "-c:a aac", "-pix_fmt yuv420p", "-movflags +faststart"} {
		if !strings.Contains(a, want) {
			t.Errorf("args lack %q: %s", want, a)
		}
	}
	if b := strings.Join(ffmpegArgs("in.mp4", "", "out.mp4", false), " "); !strings.Contains(b, "-an") || strings.Contains(b, "-vf") || strings.Contains(b, "aac") {
		t.Errorf("no audio, no filter: %s", b)
	}
}

// movingClip makes a clip with something moving and a tone: seconds long, at the size and rate, with sound or not.
func movingClip(t *testing.T, w, h, fps int, seconds float64, audio bool) string {
	t.Helper()
	if ok, _ := ffmpegAvailable(); !ok {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	args := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=" + itoa(w) + "x" + itoa(h) + ":rate=" + itoa(fps) + ":duration=" + ftoa(seconds)}
	if audio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration="+ftoa(seconds), "-c:a", "aac")
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-shortest", path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("cannot make the clip: %v: %s", err, out)
	}
	return path
}

func itoa(n int) string     { return strconv.Itoa(n) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// testResult is a Result with the folder it was left in.
type testResult struct {
	Result
	dir string
}

func runTool(t *testing.T, c *Catalog, tool, src string, settings map[string]any) (testResult, string) {
	t.Helper()
	validated, err := ValidateSettings(tool, settings)
	if err != nil {
		t.Fatal(err)
	}
	impl, ok := c.ImplOf(tool)
	if !ok {
		t.Fatalf("%s has no code", tool)
	}
	out := t.TempDir()
	var warned []string
	res, err := impl.Run(context.Background(), Job{Dir: out, OutDir: out, Video: src, Settings: validated, Warn: func(m string) { warned = append(warned, m) }})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if _, err := os.Stat(filepath.Join(out, res.File)); err != nil {
		t.Fatalf("%s left no result: %v", tool, err)
	}
	return testResult{Result: res, dir: out}, strings.Join(warned, "; ")
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestScale_RealClip_SizeLengthRateAndSoundAreAsTheSpecSays(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 832, 480, 16, 2, true)
	for _, factor := range []float64{2, 4} {
		res, _ := runTool(t, c, Scale, src, map[string]any{"factor": factor})
		if res.Width != int(832*factor) || res.Height != int(480*factor) || res.FPS != 16 || !near(res.Duration, 2, 0.15) || !res.HasAudio || res.Format != "mp4" || res.Engine != "fast" {
			t.Errorf("x%g: %+v", factor, res)
		}
	}
}

func TestScale_RealClip_QualityEngineFallsBackAndSaysSo(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 320, 180, 16, 1, false)
	res, warned := runTool(t, c, Scale, src, map[string]any{"engine": "quality"})
	if res.Engine != "fast" || len(res.Notes) != 1 || !strings.Contains(warned, "not installed") || res.HasAudio {
		t.Errorf("res = %+v, warned %q", res, warned)
	}
}

func TestInterpolate_RealClip_RaisesTheRateKeepingTheLength(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 320, 180, 16, 2, true)
	res, _ := runTool(t, c, Interpolate, src, map[string]any{"fps": 30})
	if res.FPS != 30 || !near(res.Duration, 2, 0.15) || res.Width != 320 || res.Height != 180 || res.HasAudio || res.Engine != "minterpolate" {
		t.Errorf("res = %+v", res)
	}
}

func TestInterpolate_RealClip_ATargetNotAboveIsRefused(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 320, 180, 30, 1, false)
	impl, _ := c.ImplOf(Interpolate)
	st, _ := ValidateSettings(Interpolate, map[string]any{"fps": 24})
	out := t.TempDir()
	if _, err := impl.Run(context.Background(), Job{OutDir: out, Video: src, Settings: st, Warn: func(string) {}}); err == nil || !strings.Contains(err.Error(), "not above") {
		t.Errorf("err = %v", err)
	}
}

func TestReframe_RealClip_EveryListedAspectRatioByCropAndByPad(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 640, 360, 16, 1, true)
	for _, ratio := range []string{"16:9", "9:16", "1:1", "4:5"} {
		want, _ := aspectOf(ratio)
		for _, mode := range []string{"crop", "pad"} {
			res, _ := runTool(t, c, Reframe, src, map[string]any{"aspect_ratio": ratio, "mode": mode})
			got := float64(res.Width) / float64(res.Height)
			if !near(got, want, 0.01) || res.FPS != 16 || !near(res.Duration, 1, 0.15) || !res.HasAudio {
				t.Errorf("%s %s: %dx%d (%.3f, want %.3f) %+v", ratio, mode, res.Width, res.Height, got, want, res)
			}
			if mode == "crop" && (res.Width > 640 || res.Height > 360) {
				t.Errorf("%s crop is larger than the source: %dx%d", ratio, res.Width, res.Height)
			}
			if mode == "pad" && (res.Width < 640 || res.Height < 360) {
				t.Errorf("%s pad cuts the picture: %dx%d", ratio, res.Width, res.Height)
			}
		}
	}
}
