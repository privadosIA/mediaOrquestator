package music

import (
	"math"
	"math/rand"
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
