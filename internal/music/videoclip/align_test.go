package videoclip

import (
	"math"
	"testing"
)

// song builds an envelope: level changes at the given seconds (quiet, loud, quiet, ...), starting at 0.2.
func songEnv(total float64, changes ...float64) []float64 {
	n := int(total / EnvelopeHop)
	env := make([]float64, n)
	level := 0.2
	next := 0
	for i := range env {
		t := float64(i) * EnvelopeHop
		for next < len(changes) && t >= changes[next] {
			if level == 0.2 {
				level = 0.8
			} else {
				level = 0.2
			}
			next++
		}
		env[i] = level
	}
	return env
}

func TestRefineBoundariesMovesToTheChangeOfLoudness(t *testing.T) {
	// the text puts the chorus at 26 s, the music brings it in at 30 s
	env := songEnv(90, 30, 60)
	got, moved := RefineBoundaries([]float64{26, 63}, env, 90)
	if moved != 2 {
		t.Fatalf("moved %d: %v", moved, got)
	}
	if math.Abs(got[0]-30) > 0.5 || math.Abs(got[1]-60) > 0.5 {
		t.Fatalf("boundaries = %v, want about 30 and 60", got)
	}
}

func TestRefineBoundariesLeavesAnEvenSongAlone(t *testing.T) {
	env := make([]float64, 360)
	for i := range env {
		env[i] = 0.5 + 0.02*math.Sin(float64(i)) // no section change anywhere
	}
	got, moved := RefineBoundaries([]float64{30, 60}, env, 90)
	if moved != 0 || got[0] != 30 || got[1] != 60 {
		t.Fatalf("got %v moved %d", got, moved)
	}
}

func TestRefineBoundariesDoesNotChaseAChangeThatIsTooFar(t *testing.T) {
	env := songEnv(90, 45) // the only change is 19 s from the estimate: not the same section change
	got, moved := RefineBoundaries([]float64{26}, env, 90)
	if moved != 0 || got[0] != 26 {
		t.Fatalf("got %v moved %d", got, moved)
	}
}

func TestRefineBoundariesKeepsTheOrderAndIgnoresNoEnvelope(t *testing.T) {
	got, moved := RefineBoundaries([]float64{10, 12}, nil, 30)
	if moved != 0 || got[0] != 10 || got[1] != 12 {
		t.Fatalf("got %v", got)
	}
	env := songEnv(40, 11)
	got, _ = RefineBoundaries([]float64{10, 12}, env, 40)
	if !(got[0] < got[1]) {
		t.Fatalf("boundaries out of order: %v", got)
	}
}

func TestBuildAlignedCutsAtTheChange(t *testing.T) {
	lyrics := "[Verse]\nuna linea de la estrofa\notra linea de la estrofa\n[Chorus]\nuna linea del estribillo\notra linea del estribillo"
	plain, err := Build(Options{Lyrics: lyrics, AudioSeconds: 60, ShotSeconds: 6, TransitionSeconds: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	// the music changes at 34 s, the text split puts the chorus at 30 s
	env := songEnv(60, 34)
	shots, moved, err := BuildAligned(Options{Lyrics: lyrics, AudioSeconds: 60, ShotSeconds: 6, TransitionSeconds: 0.5, Envelope: env})
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d", moved)
	}
	firstChorus := func(sh []Shot) float64 {
		for _, s := range sh {
			if s.Section == "Chorus" {
				return s.Start
			}
		}
		return -1
	}
	if got := firstChorus(shots); math.Abs(got-34) > 0.5 {
		t.Fatalf("the chorus starts at %.1f, want about 34 (without the song: %.1f)", got, firstChorus(plain))
	}
	// the video still ends exactly where the song does, and the shots keep their order and add up
	var sum float64
	for i, s := range shots {
		sum += s.Seconds
		if i > 0 && s.Start <= shots[i-1].Start {
			t.Fatalf("shots out of order at %d", i)
		}
	}
	if math.Abs(sum-60) > 1e-6 {
		t.Fatalf("shots add up to %.3f", sum)
	}
}
