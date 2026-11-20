package motionref

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// wordsGraph is a plain ground with a word swapped every second (two words, alternating), with no cut in it.
const wordsGraph = "color=c=0x203040:s=320x240:r=10:d=%d,drawtext=fontcolor=white:fontsize=90:x=(w-tw)/2:y=(h-th)/2:text='HELLO':enable='lt(mod(t\\,2)\\,1)'," +
	"drawtext=fontcolor=white:fontsize=90:x=(w-tw)/2:y=(h-th)/2:text='WORLD':enable='gte(mod(t\\,2)\\,1)'"

func TestChanges_AWordSwappedOnTheSameGroundIsAShot(t *testing.T) {
	needTools(t)
	video := makeVideo(t, sprintf(wordsGraph, 8))
	a := &Analyser{}
	times, err := a.changeTimes(context.Background(), video, 8)
	if err != nil {
		t.Fatal(err)
	}
	// the swaps are at 1, 2, ... 7 s; each is found at its frame, within 0.1 s
	if len(times) != 7 {
		t.Fatalf("changes %v, want 7 (one a second)", times)
	}
	for i, at := range times {
		if math.Abs(at-float64(i+1)) > 0.1 {
			t.Fatalf("change %d at %v, want about %d", i, at, i+1)
		}
	}
	an := &Analysis{Duration: 8}
	if err := a.Breakdown(context.Background(), video, an); err != nil {
		t.Fatal(err)
	}
	if len(an.Shots) != 8 || an.Cuts != 7 {
		t.Fatalf("%d shots, %d cuts: %+v", len(an.Shots), an.Cuts, an.Shots)
	}
	for i, s := range an.Shots {
		want := KindChange
		if i == 0 {
			want = ""
		}
		if s.Kind != want {
			t.Fatalf("shot %d kind %q, want %q", i, s.Kind, want)
		}
	}
	if an.Rhythm.HardCutShare != 0 || an.Rhythm.ChangesPerMinute[1] == 0 || an.Rhythm.CutsPerMinute[1] != an.Rhythm.ChangesPerMinute[1] {
		t.Fatalf("every boundary is a change: %+v", an.Rhythm)
	}
	if want := "Cuts and text changes: "; an.RhythmText[:len(want)] != want {
		t.Fatalf("the text names the kinds: %s", an.RhythmText)
	}
	// the same video, the same analysis
	again := &Analysis{Duration: 8}
	if err := a.Breakdown(context.Background(), video, again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(an.Shots, again.Shots) || an.RhythmText != again.RhythmText {
		t.Fatal("two runs differ")
	}
}

func TestChanges_ASlowPanHasNone(t *testing.T) {
	needTools(t)
	// the word crosses the picture at 8 px a second for ten seconds: no swap, no change
	video := makeVideo(t, "color=c=0x203040:s=320x240:r=10:d=10,drawtext=fontcolor=white:fontsize=90:x='20+t*8':y=(h-th)/2:text='HELLO'")
	times, err := (&Analyser{}).changeTimes(context.Background(), video, 10)
	if err != nil || len(times) != 0 {
		t.Fatalf("%v %v", err, times)
	}
}

func TestChanges_AHardCutAndAChangeRightAfterAreOneBoundary(t *testing.T) {
	needTools(t)
	// red then blue at 3 s with a word appearing at 3.1 s
	path := filepath.Join(t.TempDir(), "v.mp4")
	args := []string{"-y", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=320x240:r=10:d=3", "-f", "lavfi", "-i", "color=c=blue:s=320x240:r=10:d=3",
		"-filter_complex", "[0][1]concat=n=2:v=1:a=0,drawtext=fontcolor=white:fontsize=90:x=(w-tw)/2:y=(h-th)/2:text='HELLO':enable='gte(t\\,3.1)'",
		"-pix_fmt", "yuv420p", path}
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	an := &Analysis{Duration: 6}
	if err := (&Analyser{}).Breakdown(context.Background(), path, an); err != nil {
		t.Fatal(err)
	}
	if len(an.Shots) != 2 || an.Shots[1].Kind != KindCut {
		t.Fatalf("one boundary, a cut: %+v", an.Shots)
	}
}

func TestBoundaries_AChangeNextToACutIsTheCut(t *testing.T) {
	got := boundaries([]float64{3, 10}, []float64{3.2, 5, 9.8, 12})
	want := []boundary{{3, KindCut}, {5, KindChange}, {10, KindCut}, {12, KindChange}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

func TestShotsFromBoundaries_MergeTheShortOnesIntoTheBeforeAndKeepItsKind(t *testing.T) {
	// a change at 3 and a cut at 3.2 (a flash), a change at 6 and a last cut 0.2 s before the end
	got := shotsFromBoundaries([]boundary{{3, KindChange}, {3.2, KindCut}, {6, KindChange}, {8.8, KindCut}}, 9)
	var starts []float64
	var kinds []string
	for _, s := range got {
		starts = append(starts, s.Start)
		kinds = append(kinds, s.Kind)
	}
	if want := []float64{0, 3.2, 6}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("starts %v, want %v", starts, want)
	}
	if want := []string{"", KindCut, KindChange}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	if got[len(got)-1].End != 9 || got[0].End != 3.2 {
		t.Fatalf("%+v", got)
	}
	if one := shotsFromBoundaries(nil, 6); len(one) != 1 || one[0].Start != 0 || one[0].End != 6 || one[0].Kind != "" {
		t.Fatalf("%+v", one)
	}
}

func TestChangesFromDiffs_TheThresholdFollowsTheNoiseOfTheVideo(t *testing.T) {
	quiet := []float64{0.001, 0.001, 0.2, 0.001, 0.001, 0.001, 0.07, 0.001}
	// a quiet video: the floor (0.06) decides, so both the 0.2 and the 0.07 are changes
	if got := changesFromDiffs(quiet, changeK); !reflect.DeepEqual(got, []int{2, 6}) {
		t.Fatalf("%v", got)
	}
	// a noisy one (median 0.04): 4 x the noise is 0.16, only the 0.2 is a change
	noisy := []float64{0.04, 0.04, 0.2, 0.04, 0.04, 0.04, 0.07, 0.04, 0.04}
	if got := changesFromDiffs(noisy, changeK); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("%v", got)
	}
	// a plateau gives one change, not two; no pass at k of 0
	if got := changesFromDiffs([]float64{0, 0, 0.3, 0.3, 0, 0}, changeK); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if got := changesFromDiffs(quiet, 0); got != nil {
		t.Fatalf("%v", got)
	}
}

func TestChangeSignal_TheFineDifferencesPutTheChangeAtItsFrame(t *testing.T) {
	perCoarse := fineRate / changeRate
	fine := make([]float64, 6*perCoarse)
	coarse := make([]float64, 6)
	coarse[2] = 0.2
	fine[2*perCoarse+7] = 0.2 // the jump is between the pictures 7 and 8 of the interval that starts at 1.0 s
	sig := changeSignal{Coarse: coarse, Fine: fine}
	got := sig.times(changeK)
	if want := float64(2*perCoarse+8) / fineRate; len(got) != 1 || math.Abs(got[0]-want) > 1e-9 {
		t.Fatalf("%v, want [%v]", got, want)
	}
	// without the fine differences (a long video) the change stays at the middle of its interval
	if got := (changeSignal{Coarse: coarse}).times(changeK); len(got) != 1 || got[0] != 1.25 {
		t.Fatalf("%v", got)
	}
}

func TestShotPasses_RaiseTheChangeThresholdThenDropItThenRaiseTheScene(t *testing.T) {
	want := []shotPass{{0.3, 3}, {0.3, 5}, {0.3, 7}, {0.3, 9}, {0.3, 0}, {0.4, 0}, {0.5, 0}}
	if got := shotPasses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

// offBeat is the click track at 120 bpm (a beat every 0.5 s) under a video that changes colour every 2 s, always 0.25 s
// after a beat: nine boundaries, none on the tracker's beats but all on the half beat.
func offBeat(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.mp4")
	args := []string{"-y", "-v", "error", "-i", withClicks(t, 120)}
	lengths := []string{"2.25", "2", "2", "2", "2", "2", "2", "2", "2", "1.75"}
	graph := ""
	for i, l := range lengths {
		c := "red"
		if i%2 == 1 {
			c = "blue"
		}
		args = append(args, "-f", "lavfi", "-i", "color=c="+c+":s=160x120:r=10:d="+l)
		graph += "[" + string(rune('1'+i)) + "]"
	}
	args = append(args, "-filter_complex", graph+"concat=n=10:v=1:a=0[v]", "-map", "[v]", "-map", "0:a", "-c:a", "aac", "-shortest", "-pix_fmt", "yuv420p", path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	return path
}

func TestBeatPhase_CutsOnTheOffBeatOfTheTrackerReadAsOnTheBeat(t *testing.T) {
	needTools(t)
	tempo := 120.0
	an := &Analysis{Duration: 20, Tempo: &tempo}
	if err := (&Analyser{}).Breakdown(context.Background(), offBeat(t), an); err != nil {
		t.Fatal(err)
	}
	if len(an.Shots) != 10 {
		t.Fatalf("%d shots: %+v", len(an.Shots), an.Shots)
	}
	if an.Rhythm.BeatPhase != 0.5 {
		t.Fatalf("phase %v, want 0.5", an.Rhythm.BeatPhase)
	}
	if an.Rhythm.BeatCutShare == nil || *an.Rhythm.BeatCutShare != 1 {
		t.Fatalf("share %v: every boundary is on the shifted beat", an.Rhythm.BeatCutShare)
	}
	if want := "100 % of cuts on the beat (half a beat off the tracked grid)"; !contains(an.RhythmText, want) {
		t.Fatalf("%s lacks %q", an.RhythmText, want)
	}
}

func TestShiftBeats_FollowsTheLocalGap(t *testing.T) {
	got := shiftBeats([]float64{1, 2, 4}, 0.5)
	if want := []float64{1.5, 3, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }
