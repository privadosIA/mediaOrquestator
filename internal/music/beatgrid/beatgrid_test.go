package beatgrid

import (
	"math"
	"math/rand"
	"testing"
)

const testRate = 8000

// clicks is a pulse: a short decaying burst every 60/bpm seconds from first,
// over a little noise, for the given seconds.
func clicks(seconds, bpm, first float64) []float32 {
	rng := rand.New(rand.NewSource(1))
	out := make([]float32, int(seconds*testRate))
	for i := range out {
		out[i] = float32(rng.NormFloat64() * 0.005)
	}
	period := 60 / bpm
	for t := first; t < seconds; t += period {
		start := int(t * testRate)
		for j := 0; j < int(0.03*testRate) && start+j < len(out); j++ {
			out[start+j] += float32(0.8 * math.Exp(-float64(j)/60) * math.Sin(2*math.Pi*110*float64(j)/testRate))
		}
	}
	return out
}

func TestBeatPhaseFindsWhereThePulseFalls(t *testing.T) {
	for _, c := range []struct{ bpm, first float64 }{{120, 0.23}, {96, 0.41}, {75, 0.05}} {
		got, ok := Phase(clicks(40, c.bpm, c.first), testRate, c.bpm)
		if !ok {
			t.Fatalf("%v bpm: no pulse found", c.bpm)
		}
		if d := math.Abs(got - c.first); d > 0.02 {
			t.Errorf("%v bpm starting at %v s: phase = %v", c.bpm, c.first, got)
		}
	}
}

func TestBeatPhaseIgnoresALeadingSilenceAndFindsTheFirstBeatInTheBeat(t *testing.T) {
	// the first click is 1.3 s in: the pulse is still at 1.3 mod 0.5 = 0.3 s
	got, ok := Phase(clicks(40, 120, 1.3), testRate, 120)
	if !ok || math.Abs(got-0.3) > 0.02 {
		t.Fatalf("phase = %v ok=%v, want 0.3", got, ok)
	}
}

func TestBeatPhaseSaysNothingWhenThereIsNoPulse(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	noise := make([]float32, 40*testRate)
	for i := range noise {
		noise[i] = float32(rng.NormFloat64() * 0.1)
	}
	if _, ok := Phase(noise, testRate, 120); ok {
		t.Error("steady noise has no pulse")
	}
	if _, ok := Phase(make([]float32, 40*testRate), testRate, 120); ok {
		t.Error("silence has no pulse")
	}
	if _, ok := Phase(clicks(1, 120, 0.1), testRate, 120); ok {
		t.Error("one second says nothing")
	}
	for _, bpm := range []float64{0, 10, 500} {
		if _, ok := Phase(clicks(40, 120, 0.1), testRate, bpm); ok {
			t.Errorf("%v bpm is not a tempo", bpm)
		}
	}
}

func TestBeatPhaseToleratesATempoThatIsAFewPercentOff(t *testing.T) {
	// the engine's bpm is approximate: 120 asked, 119 in the audio
	got, ok := Phase(clicks(40, 119, 0.2), testRate, 120)
	if !ok || math.Abs(got-0.2) > 0.05 {
		t.Fatalf("phase = %v ok=%v, want about 0.2", got, ok)
	}
}

// drifting is a pulse whose tempo wanders: the beats fall where the cumulative sum of slightly changing periods puts them.
func drifting(seconds, bpm, first float64) ([]float32, []float64) {
	rng := rand.New(rand.NewSource(3))
	out := make([]float32, int(seconds*testRate))
	for i := range out {
		out[i] = float32(rng.NormFloat64() * 0.005)
	}
	var beats []float64
	t := first
	for t < seconds-0.1 {
		beats = append(beats, t)
		start := int(t * testRate)
		for j := 0; j < int(0.03*testRate) && start+j < len(out); j++ {
			out[start+j] += float32(0.8 * math.Exp(-float64(j)/60) * math.Sin(2*math.Pi*110*float64(j)/testRate))
		}
		// the tempo slowly drifts between 97% and 103% of the stated one
		t += 60 / bpm * (1 + 0.03*math.Sin(t/9))
	}
	return out, beats
}

func TestTrackFollowsAPulseThatDrifts(t *testing.T) {
	samples, truth := drifting(90, 120, 0.4)
	tr, ok := Track(samples, testRate, 120)
	if !ok || len(tr.Beats) < 150 {
		t.Fatalf("ok=%v beats=%d", ok, len(tr.Beats))
	}
	// Each real beat has a tracked one near it: the grid must not drift away from the pulse.
	worst, sum := 0.0, 0.0
	for _, b := range truth[:len(truth)-3] {
		best := math.Inf(1)
		for _, g := range tr.Beats {
			best = math.Min(best, math.Abs(g-b))
		}
		worst, sum = math.Max(worst, best), sum+best
	}
	if mean := sum / float64(len(truth)-3); mean > 0.04 || worst > 0.12 {
		t.Fatalf("the tracked beats are %.0f ms off on average and %.0f ms at worst", mean*1000, worst*1000)
	}
	// A plain grid at the stated tempo is far off by the end: that is what the tracking is for.
	plain := 0.0
	for _, b := range truth {
		best := math.Inf(1)
		for k := 0; k < 400; k++ {
			best = math.Min(best, math.Abs(0.4+float64(k)*0.5-b))
		}
		plain = math.Max(plain, best)
	}
	if plain < 0.15 {
		t.Fatalf("the test pulse should drift from a plain grid, it is only %.0f ms off", plain*1000)
	}
}

func TestTrackCorrectsAStatedTempoThatIsNotTheRealOne(t *testing.T) {
	samples := clicks(60, 117.7, 0.3)
	tr, ok := Track(samples, testRate, 120) // the song was asked for 120 and came out at 117.7
	if !ok || math.Abs(tr.BPM-117.7) > 0.8 {
		t.Fatalf("ok=%v bpm=%v", ok, tr.BPM)
	}
}

func TestTrackSaysNothingWhenThereIsNoPulse(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	noise := make([]float32, 40*testRate)
	for i := range noise {
		noise[i] = float32(rng.NormFloat64() * 0.1)
	}
	if _, ok := Track(noise, testRate, 120); ok {
		t.Fatal("noise has no pulse")
	}
	if _, ok := Track(make([]float32, 40*testRate), testRate, 120); ok {
		t.Fatal("silence has no pulse")
	}
	if _, ok := Track(clicks(1, 120, 0.1), testRate, 120); ok {
		t.Fatal("a second says nothing")
	}
}
