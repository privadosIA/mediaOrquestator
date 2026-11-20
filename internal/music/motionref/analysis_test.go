package motionref

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func needTools(t *testing.T) {
	t.Helper()
	for _, b := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s is not installed", b)
		}
	}
}

// makeVideo writes a video from an ffmpeg filter graph (lavfi) and returns its path.
func makeVideo(t *testing.T, graph string, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.mp4")
	args := append([]string{"-y", "-v", "error", "-f", "lavfi", "-i", graph}, extra...)
	args = append(args, "-pix_fmt", "yuv420p", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	return path
}

const changing = "color=c=black:s=320x240:r=10:d=12,geq=r='255*abs(sin(T))':g='255*abs(sin(T*1.7))':b='255*abs(sin(T*2.3))'"

func TestTheKeyFramesAreSpreadAndNoneIsAlmostEqualToItsNeighbour(t *testing.T) {
	needTools(t)
	video := makeVideo(t, changing)
	dir := filepath.Join(t.TempDir(), "frames")
	an, err := (&Analyser{}).Analyse(context.Background(), video, dir)
	if err != nil {
		t.Fatal(err)
	}
	if an.Width != 320 || an.Height != 240 || an.FPS != 10 || an.Duration < 11.5 || an.Duration > 12.5 {
		t.Fatalf("%+v", an)
	}
	if len(an.Frames) < MinFrames || len(an.Frames) > MaxFrames {
		t.Fatalf("%d frames", len(an.Frames))
	}
	var prev []float64
	for i, name := range an.Frames {
		sig, err := signature(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if prev != nil && distance(prev, sig) < sameFrame {
			t.Fatalf("frame %d is almost equal to the one before", i)
		}
		prev = sig
	}
	if entries, _ := os.ReadDir(dir); len(entries) != len(an.Frames) {
		t.Fatalf("only the key frames stay in the folder: %d files for %d frames", len(entries), len(an.Frames))
	}
}

func TestAFlatVideoKeepsTheMinimumOfFrames(t *testing.T) {
	needTools(t)
	video := makeVideo(t, "color=c=blue:s=160x120:r=5:d=20")
	an, err := (&Analyser{}).Analyse(context.Background(), video, filepath.Join(t.TempDir(), "f"))
	if err != nil || len(an.Frames) != MinFrames {
		t.Fatalf("%v %+v", err, an)
	}
}

func TestSomethingThatIsNotAVideoIsRefused(t *testing.T) {
	needTools(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(path, []byte("hello"), 0o644)
	if _, err := (&Analyser{}).Analyse(context.Background(), path, t.TempDir()); err == nil {
		t.Fatal("a text file is not a video")
	}
}

// threeShots is a video of three flat shots (red, white, blue) of 3 s each: two cuts.
func threeShots(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "three.mp4")
	args := []string{"-y", "-v", "error"}
	for _, c := range []string{"red", "white", "blue"} {
		args = append(args, "-f", "lavfi", "-i", "color=c="+c+":s=320x240:r=10:d=3")
	}
	args = append(args, "-filter_complex", "[0][1][2]concat=n=3:v=1:a=0", "-pix_fmt", "yuv420p", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	return path
}

// near says whether a hex colour is close to the pure colour (r, g, b of 0 or 255) it should be.
func near(hex string, r, g, b int) bool {
	var hr, hg, hb int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &hr, &hg, &hb); err != nil {
		return false
	}
	d := func(a, b int) int {
		if a > b {
			return a - b
		}
		return b - a
	}
	return d(hr, r) < 60 && d(hg, g) < 60 && d(hb, b) < 60
}

func TestTheColoursAndTheCutsOfAVideoWithKnownShots(t *testing.T) {
	needTools(t)
	video := threeShots(t)
	an, err := (&Analyser{}).Analyse(context.Background(), video, filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	if an.Cuts != 2 || an.MeanShot < 2.9 || an.MeanShot > 3.1 {
		t.Fatalf("cuts %d, mean shot %v", an.Cuts, an.MeanShot)
	}
	if len(an.Colours) != 3 {
		t.Fatalf("three flat colours: %+v", an.Colours)
	}
	got := map[string]bool{}
	for _, c := range an.Colours {
		if c.Share < 0.3 || c.Share > 0.37 {
			t.Fatalf("each colour covers a third: %+v", an.Colours)
		}
		switch {
		case near(c.Hex, 255, 0, 0):
			got["red"] = true
		case near(c.Hex, 255, 255, 255):
			got["white"] = true
		case near(c.Hex, 0, 0, 255):
			got["blue"] = true
		}
	}
	if len(got) != 3 {
		t.Fatalf("red, white and blue are expected: %+v", an.Colours)
	}
	if len(an.Thirds) != 3 || len(an.Thirds[0]) != 1 || !near(an.Thirds[0][0].Hex, 255, 0, 0) || !near(an.Thirds[2][0].Hex, 0, 0, 255) {
		t.Fatalf("a colour for each third: %+v", an.Thirds)
	}
}

func TestTheSameVideoGivesTheSameAnalysis(t *testing.T) {
	needTools(t)
	video := threeShots(t)
	a, err1 := (&Analyser{}).Analyse(context.Background(), video, filepath.Join(t.TempDir(), "a"))
	b, err2 := (&Analyser{}).Analyse(context.Background(), video, filepath.Join(t.TempDir(), "b"))
	if err1 != nil || err2 != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("%v %v\n%+v\n%+v", err1, err2, a, b)
	}
}

func TestAMostlyOneColourVideoStillGivesColours(t *testing.T) {
	needTools(t)
	an, err := (&Analyser{}).Analyse(context.Background(), makeVideo(t, "color=c=0x202020:s=160x120:r=5:d=6"), filepath.Join(t.TempDir(), "f"))
	if err != nil || len(an.Colours) != 1 || an.Colours[0].Share != 1 || an.Cuts != 0 || an.MeanShot != 6 {
		t.Fatalf("%v %+v", err, an)
	}
}

// withClicks is a video of 20 s with a click track at the tempo (a decaying noise burst on every beat).
func withClicks(t *testing.T, bpm float64) string {
	t.Helper()
	dir := t.TempDir()
	const rate = 22050
	pcm := make([]byte, 0, 20*rate*4)
	rng := rand.New(rand.NewSource(1))
	samples := make([]float32, 20*rate)
	step := 60 / bpm * rate
	for beat := 0.0; beat*step < float64(len(samples)); beat++ {
		start := int(beat * step)
		for i := 0; i < 2000 && start+i < len(samples); i++ {
			samples[start+i] += float32((rng.Float64()*2 - 1) * math.Exp(-float64(i)/300))
		}
	}
	for _, v := range samples {
		pcm = binary.LittleEndian.AppendUint32(pcm, math.Float32bits(v))
	}
	raw := filepath.Join(dir, "a.pcm")
	os.WriteFile(raw, pcm, 0o644)
	path := filepath.Join(dir, "v.mp4")
	if b, err := exec.Command("ffmpeg", "-y", "-v", "error", "-f", "lavfi", "-i", "color=c=gray:s=160x120:r=5:d=20", "-f", "f32le", "-ar", "22050", "-ac", "1", "-i", raw,
		"-c:a", "aac", "-pix_fmt", "yuv420p", "-shortest", path).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	return path
}

func TestTheTempoOfTheSoundIsMeasuredAndAVideoWithoutSoundHasNone(t *testing.T) {
	needTools(t)
	an, err := (&Analyser{}).Analyse(context.Background(), withClicks(t, 120), filepath.Join(t.TempDir(), "f"))
	if err != nil || an.Tempo == nil || math.Abs(*an.Tempo-120) > 3 {
		t.Fatalf("%v %+v", err, an)
	}
	silent, err := (&Analyser{}).Analyse(context.Background(), makeVideo(t, "color=c=gray:s=160x120:r=5:d=20"), filepath.Join(t.TempDir(), "g"))
	if err != nil || silent.Tempo != nil || silent.Key != nil || len(silent.Frames) == 0 {
		t.Fatalf("no sound: the tempo and the key are absent and the rest is there: %v %+v", err, silent)
	}
}
