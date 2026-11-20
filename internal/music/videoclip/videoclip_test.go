package videoclip

import (
	"math"
	"strings"
	"testing"
)

const lyrics = `[Verse]
Walking down the road under the morning sun
Nothing left to carry and nowhere to run
[Chorus]
Sing it loud, sing it now
We are free, we are free
[Verse]
Every little step is a story to tell
Every little silence rings like a bell
[Chorus]
Sing it loud, sing it now
We are free, we are free`

func TestParseLyrics_SplitsByTagAndDropsBlankLines(t *testing.T) {
	s := ParseLyrics("intro line\n\n[Verse 1]\na\nb\n[Chorus]\nc\n[Outro]\n")
	if len(s) != 4 || s[0].Tag != "" || s[1].Tag != "Verse 1" || len(s[1].Lines) != 2 || s[3].Tag != "Outro" || len(s[3].Lines) != 0 {
		t.Fatalf("sections = %+v", s)
	}
	if got := ParseLyrics(" \n\n"); len(got) != 0 {
		t.Errorf("no text, no sections: %+v", got)
	}
}

func TestBuild_TheShotsCoverTheWholeSongInOrderAndTheVideoEndsWhereItDoes(t *testing.T) {
	shots, err := Build(Options{Lyrics: lyrics, AudioSeconds: 120, ShotSeconds: 6, TransitionSeconds: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	var sum, at float64
	for i, s := range shots {
		if math.Abs(s.Start-at) > 1e-6 {
			t.Errorf("shot %d starts at %v, want %v", i, s.Start, at)
		}
		at += s.Seconds
		sum += s.Seconds
		if s.Seconds < MinShotSeconds-1e-9 {
			t.Errorf("shot %d lasts %v: too short", i, s.Seconds)
		}
		wantDur := s.Seconds
		if i < len(shots)-1 {
			wantDur += 0.8
		}
		if math.Abs(s.Duration-wantDur) > 1e-9 {
			t.Errorf("shot %d duration %v, want %v", i, s.Duration, wantDur)
		}
	}
	if math.Abs(sum-120) > 1e-6 {
		t.Errorf("the shots add up to %v s, want 120", sum)
	}
	// total of the scenes minus the overlaps is the song
	var scenes float64
	for _, s := range shots {
		scenes += s.Duration
	}
	if got := scenes - 0.8*float64(len(shots)-1); math.Abs(got-120) > 1e-6 {
		t.Errorf("the video lasts %v, want 120", got)
	}
	if len(shots) < 15 || len(shots) > 25 {
		t.Errorf("120 s at about 6 s should be about 20 shots, got %d", len(shots))
	}
}

func TestBuild_EachShotCarriesItsLinesAndTheChorusMovesMore(t *testing.T) {
	shots, _ := Build(Options{Lyrics: lyrics, AudioSeconds: 60, ShotSeconds: 6, TransitionSeconds: 0.5})
	var chorusStrong, verseSoft bool
	seen := map[string]bool{}
	for _, s := range shots {
		for _, l := range s.Lines {
			seen[l] = true
		}
		if s.Section == "Chorus" && s.Intensity > 0.5 {
			chorusStrong = true
		}
		if s.Section == "Verse" && s.Intensity < 0.4 {
			verseSoft = true
		}
		if !strings.Contains(s.ID, "-") && !strings.Contains(s.ID, "_") {
			t.Errorf("bad id %q", s.ID)
		}
	}
	if !chorusStrong || !verseSoft {
		t.Errorf("chorus strong %v verse soft %v", chorusStrong, verseSoft)
	}
	for _, l := range strings.Split(lyrics, "\n") {
		if !strings.HasPrefix(l, "[") && !seen[l] {
			t.Errorf("the line %q is in no shot", l)
		}
	}
	if shots[0].Motion == shots[1].Motion {
		t.Error("the camera should not move the same way in consecutive shots")
	}
}

func TestBuild_AnInstrumentalSongIsCutEvenlyAndALongOneGetsLongerShots(t *testing.T) {
	shots, err := Build(Options{AudioSeconds: 60, ShotSeconds: 6, TransitionSeconds: 0.5})
	if err != nil || len(shots) != 10 || shots[0].Section != "Instrumental" {
		t.Fatalf("%v %d", err, len(shots))
	}
	long, _ := Build(Options{AudioSeconds: 1200, ShotSeconds: 4, TransitionSeconds: 0.5})
	if len(long) > MaxShots {
		t.Errorf("%d shots: more than the limit", len(long))
	}
}

func TestBuild_RefusesWhatCannotBeBuilt(t *testing.T) {
	if _, err := Build(Options{AudioSeconds: 0}); err == nil {
		t.Error("a song with no length")
	}
	if _, err := Build(Options{AudioSeconds: 30, ShotSeconds: 4, TransitionSeconds: 4}); err == nil {
		t.Error("a transition as long as a shot")
	}
}

func TestFallbackPrompt_UsesTheStyleAndTheLines(t *testing.T) {
	p := FallbackPrompt("warm acoustic pop", Shot{Lines: []string{"a", "b"}})
	if !strings.Contains(p, "warm acoustic pop") || !strings.Contains(p, "a / b") {
		t.Errorf("prompt = %q", p)
	}
}
