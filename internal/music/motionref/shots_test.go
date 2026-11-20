package motionref

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

// flashed is a video of three colours of 3 s with a white flash of 0.2 s in the middle of the first one: red, a flash,
// red again, blue and green. The flash is not a shot, and the cuts that surround it make the first shot 3.2 s long.
func flashed(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.mp4")
	args := []string{"-y", "-v", "error"}
	for _, c := range [][2]string{{"red", "1.5"}, {"white", "0.2"}, {"red", "1.5"}, {"blue", "3"}, {"green", "3"}} {
		args = append(args, "-f", "lavfi", "-i", "color=c="+c[0]+":s=320x240:r=10:d="+c[1])
	}
	args = append(args, "-filter_complex", "[0][1][2][3][4]concat=n=5:v=1:a=0", "-pix_fmt", "yuv420p", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	return path
}

func TestShots_AFlashIsNotAShot(t *testing.T) {
	needTools(t)
	video := flashed(t)
	an := &Analysis{Duration: 9.2}
	if err := (&Analyser{}).Breakdown(context.Background(), video, an); err != nil {
		t.Fatal(err)
	}
	// red | flash | red | blue | green: the flash goes into the red before it, and the red after it is another shot of
	// the same colour (the scene filter sees the cut to the flash and the cut back).
	if an.Cuts != len(an.Shots)-1 {
		t.Fatalf("cuts %d for %d shots", an.Cuts, len(an.Shots))
	}
	for i, s := range an.Shots {
		if s.Index != i || s.Length() < minShot-1e-9 {
			t.Fatalf("shot %d: %+v (no shot is under %v s)", i, s, minShot)
		}
	}
	for _, s := range an.Shots {
		if s.Colour == "#ffffff" {
			t.Fatalf("the flash made a shot: %+v", an.Shots)
		}
	}
	last := an.Shots[len(an.Shots)-1]
	if math.Abs(last.End-9.2) > 0.05 || !near(last.Colour, 0, 255, 0) && !near(last.Colour, 0, 128, 0) {
		t.Fatalf("the last shot is the green one: %+v", last)
	}
	if want := round2(9.2 / float64(len(an.Shots))); an.MeanShot != want {
		t.Fatalf("mean shot %v, want %v", an.MeanShot, want)
	}
}

func TestShots_HardCutsAtKnownTimes(t *testing.T) {
	needTools(t)
	an := &Analysis{Duration: 9}
	if err := (&Analyser{}).Breakdown(context.Background(), threeShots(t), an); err != nil {
		t.Fatal(err)
	}
	if len(an.Shots) != 3 || an.Cuts != 2 {
		t.Fatalf("%+v", an.Shots)
	}
	for i, at := range []float64{0, 3, 6} {
		if math.Abs(an.Shots[i].Start-at) > 0.15 {
			t.Fatalf("shot %d starts at %v, want %v", i, an.Shots[i].Start, at)
		}
	}
	if !near(an.Shots[0].Colour, 255, 0, 0) || !near(an.Shots[1].Colour, 255, 255, 255) || !near(an.Shots[2].Colour, 0, 0, 255) {
		t.Fatalf("a colour for each shot: %+v", an.Shots)
	}
}

func TestShots_MotionOfAStillShotAndOfAMovingBar(t *testing.T) {
	needTools(t)
	// 3 s of a still colour, then 3 s with a bar that crosses the picture
	still := makeVideo(t, "color=c=0x303060:s=320x240:r=10:d=3")
	moving := makeVideo(t, "color=c=black:s=320x240:r=10:d=3,format=gray,geq=lum='255*between(X,mod(T*100,300),mod(T*100,300)+40)'")
	a := &Analyser{}
	s1 := &Shot{Start: 0, End: 3}
	if err := a.measureShot(context.Background(), still, s1); err != nil {
		t.Fatal(err)
	}
	s2 := &Shot{Start: 0, End: 3}
	if err := a.measureShot(context.Background(), moving, s2); err != nil {
		t.Fatal(err)
	}
	if s1.Motion >= 0.02 || !s1.Static() {
		t.Fatalf("a still shot: %+v", s1)
	}
	if s2.Motion <= 0.2 || s2.Static() {
		t.Fatalf("a shot with a moving bar: %+v", s2)
	}
}

func TestShots_CutsOnAndOffTheBeat(t *testing.T) {
	needTools(t)
	// a click track at 120 bpm (a beat every 0.5 s) under a video that cuts at 4.0 and 8.0 (on beats) and at 12.25
	// and 16.25 (between two beats)
	path := filepath.Join(t.TempDir(), "v.mp4")
	clicks := withClicks(t, 120)
	args := []string{"-y", "-v", "error", "-i", clicks}
	for _, c := range [][2]string{{"red", "4"}, {"white", "4"}, {"blue", "4.25"}, {"green", "4"}, {"yellow", "3.75"}} {
		args = append(args, "-f", "lavfi", "-i", "color=c="+c[0]+":s=160x120:r=10:d="+c[1])
	}
	args = append(args, "-filter_complex", "[1][2][3][4][5]concat=n=5:v=1:a=0[v]", "-map", "[v]", "-map", "0:a", "-c:a", "aac", "-shortest", "-pix_fmt", "yuv420p", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	tempo := 120.0
	an := &Analysis{Duration: 20, Tempo: &tempo}
	if err := (&Analyser{}).Breakdown(context.Background(), path, an); err != nil {
		t.Fatal(err)
	}
	if len(an.Shots) != 5 {
		t.Fatalf("%d shots: %+v", len(an.Shots), an.Shots)
	}
	if an.Shots[0].BeatCut != nil {
		t.Fatalf("the first shot has no cut: %+v", an.Shots[0])
	}
	on := func(i int) bool { return an.Shots[i].BeatCut != nil && *an.Shots[i].BeatCut }
	off := func(i int) bool { return an.Shots[i].BeatCut != nil && !*an.Shots[i].BeatCut }
	if !on(1) || !on(2) || !off(3) || !off(4) {
		var got []string
		for _, s := range an.Shots {
			b, _ := json.Marshal(s.BeatCut)
			got = append(got, string(b))
		}
		t.Fatalf("beat cuts %v, want on, on, off, off after the first shot", got)
	}
	if an.Rhythm.BeatCutShare == nil || *an.Rhythm.BeatCutShare != 0.5 {
		t.Fatalf("share %v", an.Rhythm.BeatCutShare)
	}
	if an.Rhythm.BeatPhase != 0 {
		t.Fatalf("four boundaries are too few to choose a phase: %v", an.Rhythm.BeatPhase)
	}
}

func TestShots_NoTempoNoBeatCuts(t *testing.T) {
	needTools(t)
	an := &Analysis{Duration: 9}
	if err := (&Analyser{}).Breakdown(context.Background(), threeShots(t), an); err != nil {
		t.Fatal(err)
	}
	for _, s := range an.Shots {
		if s.BeatCut != nil {
			t.Fatalf("no sound: no shot says whether it cut on a beat: %+v", s)
		}
	}
	if an.Rhythm.BeatCutShare != nil {
		t.Fatal("the sheet omits the share")
	}
}
