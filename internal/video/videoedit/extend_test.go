package videoedit

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// standIn is a maker of clips that writes a real clip with ffmpeg: a yellow one, 640x360 at 24 frames per second.
type standIn struct {
	engines []ClipEngine
	seconds float64 // the clip it makes, whatever is asked
	asked   []ClipRequest
	picture color.Color
}

func (s *standIn) Engines(context.Context) []ClipEngine { return s.engines }

func (s *standIn) MakeClip(ctx context.Context, req ClipRequest) (ClipResult, error) {
	s.asked = append(s.asked, req)
	if _, err := os.Stat(req.Picture); err != nil {
		return ClipResult{}, fmt.Errorf("the picture is not there: %w", err)
	}
	s.picture = pixelOf(nil, req.Picture)
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return ClipResult{}, err
	}
	out := filepath.Join(req.Dir, "clip.mp4")
	sec := s.seconds
	if sec == 0 {
		sec = req.Seconds
	}
	// the first frame is magenta (the picture the clip starts at), the rest yellow
	graph := fmt.Sprintf("color=c=magenta:s=640x360:r=24:d=0.0417[m];color=c=yellow:s=640x360:r=24:d=%g[y];[m][y]concat=n=2:v=1:a=0,format=yuv420p", sec)
	if b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-filter_complex", graph, "-frames:v", fmt.Sprint(int(sec*24)), "-c:v", "libx264", out).CombinedOutput(); err != nil {
		return ClipResult{}, fmt.Errorf("%w: %s", err, b)
	}
	return ClipResult{File: out, Seconds: sec}, nil
}

func both(h3, ltx bool) []ClipEngine {
	return []ClipEngine{
		{ID: EngineMiniMaxH3, Available: h3, Reason: "MiniMax H3 is not installed"},
		{ID: EngineLTX, Available: ltx, Reason: "LTX is not installed"},
	}
}

// pixelOf decodes the first pixel row middle of a picture file (png) or of a frame of a video at t seconds.
func pixelOf(t *testing.T, path string) color.Color {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path, "-frames:v", "1", "-vf", "scale=8:8", "-pix_fmt", "rgba", "-f", "rawvideo", "-").Output()
	if err != nil || len(out) < 4 {
		return color.RGBA{}
	}
	return color.RGBA{out[0], out[1], out[2], out[3]}
}

func colourNear(c color.Color, r, g, b uint8) bool {
	cr, cg, cb, _ := c.RGBA()
	d := func(a uint32, e uint8) bool {
		x := int(a>>8) - int(e)
		return x > -40 && x < 40
	}
	return d(cr, r) && d(cg, g) && d(cb, b)
}

// segments makes a 320x180 clip of 16 frames per second: one second red, one green, one blue, with a tone.
func segments(t *testing.T, audio bool) string {
	t.Helper()
	if ok, _ := ffmpegAvailable(); !ok {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), "source.mp4")
	graph := "color=c=red:s=320x180:r=16:d=1[r];color=c=green:s=320x180:r=16:d=1[g];color=c=blue:s=320x180:r=16:d=1[b];[r][g][b]concat=n=3:v=1:a=0,format=yuv420p"
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-filter_complex", graph}
	if audio {
		args = []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=duration=3", "-filter_complex", graph + "[v]", "-map", "[v]", "-map", "0:a", "-c:a", "aac", "-shortest"}
	}
	args = append(args, "-c:v", "libx264", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return path
}

func withMaker(c *Catalog, m ClipMaker) { c.SetClipMaker(m) }

func frameCount(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0", "-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var n int
	fmt.Sscan(strings.TrimSpace(string(out)), &n)
	return n
}

func TestExtend_JoinsTheSourceAndTheNewClipAtTheLastFrame(t *testing.T) {
	c := NewCatalog()
	maker := &standIn{engines: both(true, true)}
	withMaker(c, maker)
	src := segments(t, true)
	res, _ := runTool(t, c, Extend, src, map[string]any{"seconds": 4, "prompt": "she waves"})
	out := filepath.Join(res.dir, res.File)

	// the new clip was asked for with the picture the source ends on, the prompt, the engine and the shape
	if len(maker.asked) != 1 {
		t.Fatalf("clips asked = %d", len(maker.asked))
	}
	got := maker.asked[0]
	if got.Engine != EngineMiniMaxH3 || got.Prompt != "she waves" || got.Seconds != 4 || got.Width != 320 || got.Height != 180 {
		t.Errorf("asked = %+v", got)
	}
	if !colourNear(maker.picture, 0, 0, 255) {
		t.Errorf("the start picture is not the last frame (blue): %v", maker.picture)
	}
	// size and rate of the source; about 3 + 4 seconds, and no frame is shown twice: the clip has 96 frames at 24 per
	// second, drops its first and is brought to the source's 16 per second (63 or 64 frames) after the source's 48
	if res.Width != 320 || res.Height != 180 || res.FPS != 16 || res.Format != "mp4" {
		t.Errorf("res = %+v", res)
	}
	if frames := frameCount(t, out); frames < 48+62 || frames > 48+65 {
		t.Errorf("frames = %d, want about %d", frames, 48+64)
	}
	if !near(res.Duration, 7, 0.3) {
		t.Errorf("duration = %.2f", res.Duration)
	}
	// the first part is the source and the next one starts where it ends
	for _, c := range []struct {
		at      float64
		r, g, b uint8
	}{{0.5, 255, 0, 0}, {1.5, 0, 128, 0}, {2.8, 0, 0, 255}, {3.3, 255, 255, 0}, {6.5, 255, 255, 0}} {
		if px := frameAtColour(t, out, c.at); !colourNear(px, c.r, c.g, c.b) {
			t.Errorf("at %.1fs the colour is %v, want about (%d,%d,%d)", c.at, px, c.r, c.g, c.b)
		}
	}
	// the magenta first frame of the clip (the picture it started at) is not in the result
	for k := 0; k < 108; k += 5 {
		if px := frameAtColour(t, out, float64(k)/16); colourNear(px, 255, 0, 255) {
			t.Errorf("the repeated first frame of the clip shows at frame %d", k)
		}
	}
	// no sound, and the result says so; the extension is counted
	if res.HasAudio || res.ExtendedTimes != 1 || res.Engine != EngineMiniMaxH3 || len(res.Notes) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func frameAtColour(t *testing.T, video string, at float64) color.Color {
	t.Helper()
	img := frameAt(t, video, at)
	b := img.Bounds()
	return img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)
}

func TestExtend_AnExtensionOfAnExtensionIsCountedTwiceAndWarns(t *testing.T) {
	c := NewCatalog()
	maker := &standIn{engines: both(false, true)}
	withMaker(c, maker)
	first, _ := runTool(t, c, Extend, segments(t, false), map[string]any{"seconds": 2, "prompt": "one"})
	if first.ExtendedTimes != 1 || first.Engine != EngineLTX {
		t.Fatalf("first = %+v", first)
	}
	second, _ := runTool(t, c, Extend, filepath.Join(first.dir, first.File), map[string]any{"seconds": 2, "prompt": "two"})
	if second.ExtendedTimes != 2 || len(second.Notes) != 1 || !strings.Contains(second.Notes[0], "extended 2 times") {
		t.Errorf("second = %+v", second)
	}
	third, _ := runTool(t, c, Extend, filepath.Join(second.dir, second.File), map[string]any{"seconds": 2, "prompt": "three"})
	if third.ExtendedTimes != 3 {
		t.Errorf("third = %+v", third)
	}
	// a video that was never extended counts from zero, whatever comment it has
	if n := ExtendedTimes(Info{Comment: "just a comment"}); n != 0 {
		t.Errorf("n = %d", n)
	}
	if n := ExtendedTimes(Info{Comment: "x; mo-extended=7"}); n != 7 {
		t.Errorf("n = %d", n)
	}
}

func TestExtend_AShortClipGivesAShorterExtensionAndSaysSo(t *testing.T) {
	c := NewCatalog()
	maker := &standIn{engines: both(true, true), seconds: 2}
	withMaker(c, maker)
	res, warned := runTool(t, c, Extend, segments(t, false), map[string]any{"seconds": 6, "prompt": "p"})
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "lasts 2.0 s") || !strings.Contains(warned, "lasts 2.0 s") || !near(res.Duration, 5, 0.3) {
		t.Errorf("res = %+v warned %q", res, warned)
	}
}

func TestExtend_TheEngineAskedForIsUsedOrRefused(t *testing.T) {
	c := NewCatalog()
	maker := &standIn{engines: both(true, true)}
	withMaker(c, maker)
	res, _ := runTool(t, c, Extend, segments(t, false), map[string]any{"seconds": 2, "prompt": "p", "engine": "ltx"})
	if res.Engine != EngineLTX || maker.asked[0].Engine != EngineLTX {
		t.Errorf("res = %+v asked %+v", res, maker.asked)
	}
	withMaker(c, &standIn{engines: both(true, false)})
	if _, err := PickEngine(both(true, false), "ltx"); err == nil || !strings.Contains(err.Error(), "LTX is not installed") {
		t.Errorf("err = %v", err)
	}
	if _, err := PickEngine(both(true, true), "wan"); err == nil {
		t.Error("an engine that takes no first frame must be refused")
	}
}

func TestExtend_AvailabilityFollowsTheEngines(t *testing.T) {
	if ok, _ := ffmpegAvailable(); !ok {
		t.Skip("ffmpeg is not installed")
	}
	c := NewCatalog()
	withMaker(c, &standIn{engines: both(false, true)})
	if ok, why := c.extendAvailable(); !ok {
		t.Errorf("ltx alone is enough: %s", why)
	}
	withMaker(c, &standIn{engines: both(false, false)})
	ok, why := c.extendAvailable()
	if ok || !strings.Contains(why, "MiniMax H3 is not installed") || !strings.Contains(why, "LTX is not installed") {
		t.Errorf("available = %v %q", ok, why)
	}
	withMaker(c, nil)
	if ok, _ := c.extendAvailable(); ok {
		t.Error("without a maker the tool is not available")
	}
}

func TestExtend_TheSettingsAreRefusedOutsideTheirLimits(t *testing.T) {
	for _, tc := range []struct {
		settings map[string]any
		want     string
	}{
		{map[string]any{"seconds": 5}, "prompt is required"},
		{map[string]any{"seconds": 5, "prompt": "   "}, "prompt is required"},
		{map[string]any{"seconds": 1, "prompt": "p"}, "below the minimum"},
		{map[string]any{"seconds": 11, "prompt": "p"}, "above the maximum"},
		{map[string]any{"seconds": 5, "prompt": "p", "engine": "wan"}, "setting engine"},
	} {
		if _, err := ValidateSettings(Extend, tc.settings); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: %v, want %q", tc.settings, err, tc.want)
		}
	}
	if _, err := ValidateSettings(Extend, map[string]any{"prompt": "p"}); err != nil {
		t.Errorf("seconds has a default: %v", err)
	}
}

func TestExtendArgs(t *testing.T) {
	a := strings.Join(joinArgs("a.mp4", "b.mp4", "o.mp4", Info{Width: 1280, Height: 720, FPS: 29.97, FPSText: "30000/1001"}, 2), " ")
	for _, want := range []string{"trim=start_frame=1", "fps=30000/1001", "scale=1280:720:force_original_aspect_ratio=increase", "crop=1280:720", "concat=n=2:v=1:a=0", "-an", "comment=mo-extended=2"} {
		if !strings.Contains(a, want) {
			t.Errorf("args lack %q: %s", want, a)
		}
	}
	if l := strings.Join(lastFrameArgs("v.mp4", "l.png"), " "); !strings.Contains(l, "-sseof -1") || !strings.Contains(l, "-update 1") {
		t.Errorf("last frame args: %s", l)
	}
}
