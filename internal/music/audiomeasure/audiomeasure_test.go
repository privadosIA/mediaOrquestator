package audiomeasure

import (
	"math"
	"math/rand"
	"testing"
)

// clicks is a decaying noise burst on every beat of a tempo.
func clicks(bpm float64, seconds int) []float32 {
	out := make([]float32, seconds*Rate)
	rng := rand.New(rand.NewSource(1))
	step := 60 / bpm * Rate
	for beat := 0.0; beat*step < float64(len(out)); beat++ {
		start := int(beat * step)
		for i := 0; i < 2000 && start+i < len(out); i++ {
			out[start+i] += float32((rng.Float64()*2 - 1) * math.Exp(-float64(i)/300))
		}
	}
	return out
}

// chords plays triads (in semitones from a tonic at A2) one after the other.
func chords(tonic float64, triads [][3]float64, each float64, seconds int) []float32 {
	out := make([]float32, seconds*Rate)
	per := int(each * Rate)
	for i := range out {
		t := triads[(i/per)%len(triads)]
		var v float64
		for _, semi := range t {
			f := tonic * math.Pow(2, semi/12)
			for h := 1.0; h <= 3; h++ {
				v += math.Sin(2*math.Pi*f*h*float64(i)/Rate) / h
			}
		}
		out[i] = float32(v / 9)
	}
	return out
}

func TestTempoOfClicks(t *testing.T) {
	for _, want := range []float64{90, 120, 150} {
		r := Measure(clicks(want, 30))
		if r.BPM == nil {
			t.Fatalf("%v bpm: no tempo found (confidence %.2f)", want, r.BPMConfidence)
		}
		ok := math.Abs(*r.BPM-want)/want < 0.03
		for _, a := range r.BPMAlternative {
			ok = ok || math.Abs(a-want)/want < 0.03
		}
		if !ok {
			t.Errorf("%v bpm: got %v (alternatives %v)", want, *r.BPM, r.BPMAlternative)
		}
	}
}

func TestKeyOfAProgression(t *testing.T) {
	// A minor: Am - Dm - E - Am, from A2 (110 Hz).
	prog := [][3]float64{{0, 3, 7}, {5, 8, 12}, {7, 11, 14}, {0, 3, 7}}
	r := Measure(chords(110, prog, 1.0, 24))
	if r.Key == nil || (*r.Key != "A minor" && *r.Key != "C major") {
		got := "nil"
		if r.Key != nil {
			got = *r.Key
		}
		t.Fatalf("key = %s", got)
	}
}

func TestSilenceHasNoPulseOrKey(t *testing.T) {
	r := Measure(make([]float32, 20*Rate))
	if r.BPM != nil || r.Key != nil {
		t.Fatalf("silence gave %v %v", r.BPM, r.Key)
	}
}

func TestShortInputIsSafe(t *testing.T) {
	if r := Measure(make([]float32, 100)); r.BPM != nil || r.Key != nil {
		t.Fatal("a short input should measure nothing")
	}
}
