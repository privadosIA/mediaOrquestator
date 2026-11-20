package videoedit

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// spikeDir is the folder of the control frames of the motion-control spike (MOTION_CONTROL_SPIKE_DIR); the tests that
// use it are skipped when it is not set.
var spikeDir = os.Getenv("MOTION_CONTROL_SPIKE_DIR")

func TestShapeFor_ThePictureDecidesTheShapeOfTheClip(t *testing.T) {
	for _, tc := range []struct{ w, h, ew, eh int }{
		{1024, 576, 832, 480}, {1920, 1080, 832, 480}, {1500, 1000, 832, 480}, {1600, 1200, 640, 640}, {1000, 1000, 640, 640},
		{1000, 1250, 640, 640}, {1200, 1600, 640, 640}, {1000, 1500, 480, 832}, {480, 866, 480, 832}, {1072, 1936, 480, 832}, {0, 0, 832, 480},
	} {
		w, h := ShapeFor(tc.w, tc.h)
		if w != tc.ew || h != tc.eh || w%16 != 0 || h%16 != 0 {
			t.Errorf("%dx%d: %dx%d, want %dx%d", tc.w, tc.h, w, h, tc.ew, tc.eh)
		}
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func controlDir(t *testing.T, n int, make func(i int) image.Image) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		writePNG(t, filepath.Join(dir, strings.Replace("%04d.png", "%04d", pad4(i+1), 1)), make(i))
	}
	return dir
}

func pad4(i int) string {
	s := "000" + itoa(i)
	return s[len(s)-4:]
}

func rgbFrame(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func TestCheckControlDir_WhatSdNeedsAsAControlVideo(t *testing.T) {
	good := controlDir(t, 33, func(int) image.Image { return rgbFrame(64, 48) })
	if err := CheckControlDir(good); err != nil {
		t.Errorf("33 RGB frames: %v", err)
	}
	for name, tc := range map[string]struct {
		dir  string
		want string
	}{
		"empty":     {t.TempDir(), "has 0 frames"},
		"too few":   {controlDir(t, 32, func(int) image.Image { return rgbFrame(64, 48) }), "has 32 frames"},
		"too many":  {controlDir(t, 34, func(int) image.Image { return rgbFrame(64, 48) }), "has 34 frames"},
		"one grey":  {controlDir(t, 33, func(i int) image.Image { return image.NewGray(image.Rect(0, 0, 64, 48)) }), "three channels"},
		"alpha":     {controlDir(t, 33, func(i int) image.Image { return image.NewNRGBA(image.Rect(0, 0, 64, 48)) }), "three channels"},
		"one other": {controlDir(t, 33, func(i int) image.Image { return rgbFrame(64+(i/32)*16, 48) }), "the first is 64x48"},
	} {
		if err := CheckControlDir(tc.dir); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want one with %q", name, err, tc.want)
		}
	}
	if err := CheckControlDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a folder that is not there has no control")
	}
}

func TestCheckControlDir_TheSpikesControlPasses(t *testing.T) {
	if _, err := os.Stat(spikeDir); err != nil {
		t.Skip("the spike's files are not here")
	}
	for _, d := range []string{"ctrl_depthmask", "ctrl_depth", "ctrl_raw"} {
		if err := CheckControlDir(filepath.Join(spikeDir, d)); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
}

func TestWindowArgs(t *testing.T) {
	a := strings.Join(windowArgs("v.mp4", 1.5, 832, 480, "/f"), " ")
	for _, want := range []string{"-ss 1.5", "fps=16", "scale=832:480:force_original_aspect_ratio=decrease", "pad=832:480", "-frames:v 33", "/f/%04d.png"} {
		if !strings.Contains(a, want) {
			t.Errorf("args lack %q: %s", want, a)
		}
	}
}

// grey reads a PNG as a grid of 0..255 values.
func greyGrid(t *testing.T, path string) (w, h int, v []float64) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	v = make([]float64, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v = append(v, grey(img.At(x, y)))
		}
	}
	return b.Dx(), b.Dy(), v
}

// The control of the vision service on the frames of the spike looks like the control the spike made with its throwaway
// scripts: the same people cut out (the silhouettes overlap) and the same depth inside them.
func TestPrepareControl_RealService_LooksLikeTheSpikesControl(t *testing.T) {
	c := NewCatalog()
	if _, err := os.Stat(spikeDir); err != nil {
		t.Skip("the spike's files are not here")
	}
	v := realVision(t, c)
	src := filepath.Join(t.TempDir(), "spike.mp4")
	if b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-framerate", "16", "-i", filepath.Join(spikeDir, "ctrl_raw", "%04d.png"),
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "12", src).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	work := t.TempDir()
	sum, err := PrepareControl(context.Background(), v, src, 0, 832, 480, filepath.Join(work, "frames"), filepath.Join(work, "control"))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Frames != 33 || sum.FramesWithoutPeople == nil || *sum.FramesWithoutPeople > 3 {
		t.Fatalf("summary = %+v", sum)
	}
	var inter, union, absdiff, both float64
	for _, i := range []string{"0001", "0010", "0017", "0025", "0033"} {
		w1, h1, mine := greyGrid(t, filepath.Join(work, "control", i+".png"))
		w2, h2, spike := greyGrid(t, filepath.Join(spikeDir, "ctrl_depthmask", i+".png"))
		if w1 != w2 || h1 != h2 {
			t.Fatalf("size %dx%d against the spike's %dx%d", w1, h1, w2, h2)
		}
		for k := range mine {
			a, b := mine[k] > 8, spike[k] > 8
			if a && b {
				inter++
			}
			if a || b {
				union++
			}
			if a && b {
				absdiff += abs(mine[k] - spike[k])
				both++
			}
		}
	}
	if iou := inter / union; iou < 0.8 {
		t.Errorf("the silhouettes overlap %.2f, want at least 0.8", iou)
	}
	t.Logf("silhouettes overlap %.3f, mean grey difference inside them %.1f of 255", inter/union, absdiff/both)
	if mean := absdiff / both; mean > 45 {
		t.Errorf("the depth inside the people differs by %.1f of 255 from the spike's", mean)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestPrepareControl_RealService_NobodyIsRefusedBeforeAnythingIsMade(t *testing.T) {
	c := NewCatalog()
	v := realVision(t, c)
	empty := filepath.Join(t.TempDir(), "empty.mp4")
	if b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=gray:size=320x180:rate=16:duration=3", "-c:v", "libx264", "-pix_fmt", "yuv420p", empty).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	work := t.TempDir()
	_, err := PrepareControl(context.Background(), v, empty, 0, 832, 480, filepath.Join(work, "f"), filepath.Join(work, "c"))
	if !errors.Is(err, ErrNoPeople) {
		t.Errorf("err = %v, want ErrNoPeople", err)
	}
}

func TestPrepareControl_RealService_ATooShortVideoSaysHowMuchItNeeds(t *testing.T) {
	c := NewCatalog()
	v := realVision(t, c)
	short := filepath.Join(t.TempDir(), "short.mp4")
	if b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=gray:size=320x180:rate=16:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", short).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	work := t.TempDir()
	_, err := PrepareControl(context.Background(), v, short, 0, 832, 480, filepath.Join(work, "f"), filepath.Join(work, "c"))
	if err == nil || !strings.Contains(err.Error(), "needs 33") {
		t.Errorf("err = %v", err)
	}
}

func TestPrepareControl_RealService_TheWindowStartsWhereAskedAndHasTheShapeOfTheClip(t *testing.T) {
	c := NewCatalog()
	v := realVision(t, c)
	src := personClip(t, 6)
	work := t.TempDir()
	// the window is portrait, the shape a portrait picture asks for
	sum, err := PrepareControl(context.Background(), v, src, 1.5, 480, 832, filepath.Join(work, "f"), filepath.Join(work, "c"))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Frames != 33 {
		t.Fatalf("frames = %d", sum.Frames)
	}
	if err := CheckControlDir(filepath.Join(work, "c")); err != nil {
		t.Error(err)
	}
	w, h, _ := greyGrid(t, filepath.Join(work, "c", "0001.png"))
	if w != 480 || h != 832 {
		t.Errorf("control is %dx%d", w, h)
	}
}
