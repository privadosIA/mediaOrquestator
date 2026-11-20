package separate

import (
	"math"
	"reflect"
	"testing"
)

// bursts is seconds of silence with short tone bursts starting at the given times (50 ms of 1 kHz at amplitude 0.3).
func bursts(seconds float64, starts ...float64) []float32 {
	const rate = 16000
	s := make([]float32, int(seconds*rate))
	for _, st := range starts {
		for i := 0; i < int(0.05*rate); i++ {
			n := int(st*rate) + i
			if n < len(s) {
				s[n] = float32(0.3 * math.Sin(2*math.Pi*1000*float64(n)/rate))
			}
		}
	}
	return s
}

func TestAttacks_FindsTheBurstsAndNothingInTheSilence(t *testing.T) {
	starts := []float64{0.2, 0.6, 1.0, 1.4, 1.8}
	s := bursts(2.2, starts...)
	got := Attacks(s, 16000, EnvelopeOf(s, 16000))
	if len(got) != len(starts) {
		t.Fatalf("attacks = %v, want %d near %v", got, len(starts), starts)
	}
	for i, st := range starts {
		if math.Abs(got[i]-st) > 0.015 {
			t.Errorf("attack %d at %.3f, want within 15 ms of %.3f", i, got[i], st)
		}
	}
}

func TestAttacks_AreInOrderApartAndTheSameTwice(t *testing.T) {
	s := bursts(3, 0.3, 0.45, 0.9, 1.5, 1.58, 2.4)
	env := EnvelopeOf(s, 16000)
	a, b := Attacks(s, 16000, env), Attacks(s, 16000, env)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two runs differ: %v %v", a, b)
	}
	for i := 1; i < len(a); i++ {
		if a[i]-a[i-1] < MinAttackGap-1e-9 {
			t.Fatalf("attacks %v are closer than %.2f s", a, MinAttackGap)
		}
	}
}

func TestAttacks_NothingWhereThereIsNoVoice(t *testing.T) {
	s := bursts(2, 0.5, 1.2)
	silent := Env{Hop: Hop, Voiced: make([]bool, 100)} // the envelope hears no voice anywhere
	if got := Attacks(s, 16000, silent); len(got) != 0 {
		t.Fatalf("attacks with no voice: %v", got)
	}
}

func TestAttacks_ShortOrEmptyInput(t *testing.T) {
	if got := Attacks(nil, 16000, Env{}); got == nil || len(got) != 0 {
		t.Fatalf("empty input must give an empty list, got %v", got)
	}
	if got := Attacks(make([]float32, 500), 16000, Env{}); len(got) != 0 {
		t.Fatalf("a stem shorter than a frame has no attacks: %v", got)
	}
	if got := Attacks(make([]float32, 32000), 16000, Env{}); len(got) != 0 {
		t.Fatalf("silence has no attacks: %v", got)
	}
}
