package assemble

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func narratedScene() Scene {
	return Scene{ID: "s1", ImagePath: "tmp/scene_s1.png", AudioPath: "tmp/scene_s1.wav", Duration: 2.5}
}

// legacyArgs is the exact argument list the pre-camera-motion
// implementation produced, pinned as a golden value.
func legacyArgs(scene Scene, framerate int, outputPath string) []string {
	return []string{
		"-y",
		"-loop", "1",
		"-framerate", fmt.Sprintf("%d", framerate),
		"-i", scene.ImagePath,
		"-i", scene.AudioPath,
		"-t", fmt.Sprintf("%.6f", scene.Duration),
		"-c:v", "libx264",
		"-tune", "stillimage",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ar", "44100",
		"-ac", "2",
		outputPath,
	}
}

func TestBuildClipArgs_StaticIsByteIdenticalToLegacy(t *testing.T) {
	scene := narratedScene()
	want := legacyArgs(scene, 30, "tmp/scene_s1.mp4")

	for _, variant := range []Scene{
		scene, // all zero-valued extensions
		func() Scene { s := scene; s.MotionType = "static"; return s }(),
	} {
		got := BuildClipArgs(variant, 30, "tmp/scene_s1.mp4")
		if !reflect.DeepEqual(got, want) {
			t.Errorf("static args =\n  %q\nwant legacy\n  %q", got, want)
		}
	}
}

func argAfter(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestMotion_ZoomInSpansFullDuration(t *testing.T) {
	scene := narratedScene()
	scene.MotionType = "zoom_in"
	scene.MotionIntensity = 0.5
	scene.Width, scene.Height = 1280, 720

	args := BuildClipArgs(scene, 30, "out.mp4")
	vf := argAfter(t, args, "-vf")
	if vf == "" {
		t.Fatal("zoom_in must add a -vf filter")
	}
	// d = round(2.5 × 30) = 75 frames, expressions linear over on/74
	if !strings.Contains(vf, ":d=75:") {
		t.Errorf("filter should be dimensioned over 75 frames: %s", vf)
	}
	if !strings.Contains(vf, "on/74") {
		t.Errorf("zoom expression should be linear over the last frame index 74: %s", vf)
	}
	if !strings.Contains(vf, "scale=iw*2:ih*2:flags=lanczos,zoompan=") {
		t.Errorf("zoompan must be preceded by the 2x lanczos upscale: %s", vf)
	}
	if !strings.Contains(vf, "s=1280x720") || !strings.Contains(vf, "fps=30") {
		t.Errorf("output size/fps must match the configured resolution: %s", vf)
	}
	// travel = 0.08 + 0.27*0.5 = 0.215
	if !strings.Contains(vf, "1+0.2150*on") {
		t.Errorf("zoom travel should scale with intensity: %s", vf)
	}
	for _, a := range args {
		if a == "stillimage" {
			t.Error("-tune stillimage must disappear for non-static motion")
		}
	}
}

func TestMotion_PresetsProduceDistinctFilters(t *testing.T) {
	seen := map[string]string{}
	for _, mt := range []string{"zoom_in", "zoom_out", "pan_left", "pan_right", "handheld"} {
		scene := narratedScene()
		scene.MotionType = mt
		scene.MotionIntensity = 0.5
		scene.Width, scene.Height = 1280, 720
		scene.Seed = 42
		vf := BuildVideoFilter(scene, 30)
		if vf == "" {
			t.Errorf("%s produced no filter", mt)
		}
		for other, f := range seen {
			if f == vf {
				t.Errorf("%s and %s produced identical filters", mt, other)
			}
		}
		seen[mt] = vf
	}
}

func TestMotion_IntensityScalesAmplitude(t *testing.T) {
	low := narratedScene()
	low.MotionType = "zoom_in"
	low.MotionIntensity = 0.0
	low.Width, low.Height = 1280, 720
	high := low
	high.MotionIntensity = 1.0

	lowVF, highVF := BuildVideoFilter(low, 30), BuildVideoFilter(high, 30)
	if !strings.Contains(lowVF, "1+0.0800*on") {
		t.Errorf("intensity 0 should give minimum travel 0.08: %s", lowVF)
	}
	if !strings.Contains(highVF, "1+0.3500*on") {
		t.Errorf("intensity 1 should give maximum travel 0.35: %s", highVF)
	}
}

func TestMotion_HandheldDeterministicPerSeed(t *testing.T) {
	scene := narratedScene()
	scene.MotionType = "handheld"
	scene.MotionIntensity = 0.5
	scene.Width, scene.Height = 1280, 720
	scene.Seed = 42

	a := BuildVideoFilter(scene, 30)
	b := BuildVideoFilter(scene, 30)
	if a != b {
		t.Error("handheld filter must be byte-identical across calls with the same seed")
	}
	if !strings.Contains(a, "sin(") || !strings.Contains(a, "cos(") || !strings.Contains(a, "crop=") {
		t.Errorf("handheld should be sinusoidal crop jitter: %s", a)
	}
	if !strings.HasSuffix(a, "scale=1280:720") {
		t.Errorf("handheld must scale back to the configured resolution: %s", a)
	}

	scene.Seed = 43
	c := BuildVideoFilter(scene, 30)
	if c == a {
		t.Error("a different seed must produce a different jitter phase")
	}
}

func TestLook_GrainScalesAndNeutralAddsNothing(t *testing.T) {
	low := narratedScene()
	low.Grain = 0.2
	high := narratedScene()
	high.Grain = 0.8

	lowVF, highVF := BuildVideoFilter(low, 30), BuildVideoFilter(high, 30)
	if lowVF != "noise=alls=4:allf=t+u" {
		t.Errorf("grain 0.2 filter = %q, want noise=alls=4:allf=t+u", lowVF)
	}
	if highVF != "noise=alls=16:allf=t+u" {
		t.Errorf("grain 0.8 filter = %q, want noise=alls=16:allf=t+u", highVF)
	}

	neutral := narratedScene()
	neutral.Grain = 0.0
	neutral.WhiteBalance = "neutral"
	if vf := BuildVideoFilter(neutral, 30); vf != "" {
		t.Errorf("neutral look must add no filter, got %q", vf)
	}
	args := BuildClipArgs(neutral, 30, "out.mp4")
	if argAfter(t, args, "-vf") != "" {
		t.Error("neutral look must not emit a -vf flag at all")
	}
}

func TestLook_WhiteBalanceFixedTable(t *testing.T) {
	cases := map[string]string{
		"warm":        "colortemperature=temperature=4600",
		"cool":        "colortemperature=temperature=8500",
		"fluorescent": "colortemperature=temperature=4200,colorbalance=gm=.08",
	}
	for wb, want := range cases {
		scene := narratedScene()
		scene.WhiteBalance = wb
		if vf := BuildVideoFilter(scene, 30); vf != want {
			t.Errorf("white_balance %s = %q, want %q", wb, vf, want)
		}
	}
}

func TestLook_FilterOrderMotionThenWBThenGrain(t *testing.T) {
	scene := narratedScene()
	scene.MotionType = "zoom_in"
	scene.Width, scene.Height = 1280, 720
	scene.WhiteBalance = "warm"
	scene.Grain = 0.5

	vf := BuildVideoFilter(scene, 30)
	zoomIdx := strings.Index(vf, "zoompan=")
	wbIdx := strings.Index(vf, "colortemperature=")
	grainIdx := strings.Index(vf, "noise=")
	if zoomIdx == -1 || wbIdx == -1 || grainIdx == -1 {
		t.Fatalf("missing filters in chain: %s", vf)
	}
	if zoomIdx >= wbIdx || wbIdx >= grainIdx {
		t.Errorf("filter order must be motion → white balance → grain: %s", vf)
	}
}

func TestSilentClip_UsesAnullsrcWithIdenticalStreamParams(t *testing.T) {
	scene := Scene{ID: "quiet", ImagePath: "tmp/scene_quiet.png", Duration: 2.5, Silent: true}
	args := BuildClipArgs(scene, 30, "tmp/scene_quiet.mp4")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-f lavfi -i "+SilentAudioSource) {
		t.Errorf("silent clip must use the anullsrc lavfi input: %v", args)
	}
	if strings.Contains(joined, ".wav") {
		t.Errorf("silent clip must not reference a WAV input: %v", args)
	}
	if argAfter(t, args, "-t") != "2.500000" {
		t.Errorf("silent clip duration must be silence_seconds: %v", args)
	}

	// Stream-parameter flags identical to a narrated clip.
	narrated := BuildClipArgs(narratedScene(), 30, "out.mp4")
	for _, pair := range [][2]string{
		{"-c:v", "libx264"}, {"-pix_fmt", "yuv420p"},
		{"-c:a", "aac"}, {"-b:a", "192k"}, {"-ar", "44100"}, {"-ac", "2"},
	} {
		if argAfter(t, args, pair[0]) != pair[1] {
			t.Errorf("silent clip %s = %q, want %q", pair[0], argAfter(t, args, pair[0]), pair[1])
		}
		if argAfter(t, narrated, pair[0]) != pair[1] {
			t.Errorf("narrated clip %s = %q, want %q", pair[0], argAfter(t, narrated, pair[0]), pair[1])
		}
	}
	// A silent static scene keeps -tune stillimage like any static scene.
	if argAfter(t, args, "-tune") != "stillimage" {
		t.Errorf("static silent clip should keep -tune stillimage: %v", args)
	}
}

func TestRotatePresets(t *testing.T) {
	scene := Scene{ID: "s", ImagePath: "i.png", AudioPath: "a.wav", Duration: 6, MotionType: "rotate_cw", MotionIntensity: 0.5, Width: 1280, Height: 720}
	cw := strings.Join(BuildClipArgs(scene, 30, "o.mp4"), " ")
	// 25 degrees at the end (5 + 40*0.5), clockwise
	if !strings.Contains(cw, "rotate=a='0.43633*t/6.0000'") {
		t.Fatalf("clockwise: %s", cw)
	}
	if !strings.Contains(cw, "crop=1280:720") || strings.Contains(cw, "stillimage") {
		t.Fatalf("keeps the resolution and is not a still: %s", cw)
	}
	scene.MotionType = "rotate_ccw"
	if ccw := strings.Join(BuildClipArgs(scene, 30, "o.mp4"), " "); !strings.Contains(ccw, "rotate=a='-0.43633*t/6.0000'") {
		t.Fatalf("counter-clockwise: %s", ccw)
	}
	// deterministic
	if a, b := BuildVideoFilter(scene, 30), BuildVideoFilter(scene, 30); a != b {
		t.Fatal("not deterministic")
	}
}

func TestRotateZoomLeavesNoEmptyCorner(t *testing.T) {
	// A W×H frame zoomed by k and rotated by θ must still contain the output rectangle.
	for _, c := range []struct{ w, h int }{{1280, 720}, {720, 1280}, {512, 512}} {
		for _, in := range []float64{0, 0.5, 1} {
			scene := Scene{MotionType: "rotate_cw", MotionIntensity: in, Width: c.w, Height: c.h, Duration: 4}
			f := rotateFilter(scene)
			var k float64
			fmt.Sscanf(f, "scale=iw*%f", &k)
			th := RotationDegrees(in) * math.Pi / 180
			w, h := float64(c.w), float64(c.h)
			needW := w*math.Cos(th) + h*math.Sin(th)
			needH := w*math.Sin(th) + h*math.Cos(th)
			if k*w+0.5 < needW || k*h+0.5 < needH {
				t.Errorf("%dx%d intensity %v: zoom %.4f does not cover (%.1f,%.1f) from (%.1f,%.1f)", c.w, c.h, in, k, needW, needH, k*w, k*h)
			}
		}
	}
}

func TestBuildFitImageArgs(t *testing.T) {
	got := strings.Join(BuildFitImageArgs("sources/p.jpg", 1280, 720, "tmp/s.png"), " ")
	want := "-y -i sources/p.jpg -vf scale=1280:720:force_original_aspect_ratio=increase,crop=1280:720 -frames:v 1 tmp/s.png"
	if got != want {
		t.Fatalf("%s", got)
	}
}
