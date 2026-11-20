package audiomeasure

import "math"

// Temperley's key profiles, starting at the tonic. On the songs the estimator
// was checked against they found the relative key more often than the
// Krumhansl-Kessler ones, which only named a wrong key.
var (
	majorProfile = []float64{5.0, 2.0, 3.5, 2.0, 4.5, 4.0, 2.0, 4.5, 2.0, 3.5, 1.5, 4.0}
	minorProfile = []float64{5.0, 2.0, 3.5, 4.5, 2.0, 4.0, 2.0, 4.5, 3.5, 2.0, 1.5, 4.0}
	noteNames    = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
)

// chroma sums the energy of every frame into the twelve pitch classes between
// 65 Hz and 1 kHz, as linear magnitude. The low range carries the bass, which
// names the key; with the upper partials included, a note's fifth (its third
// harmonic) pulled the answer to the neighbouring key.
func chroma(samples []float32) []float64 {
	mags := magnitudes(samples, keyFFT, keyHop)
	if len(mags) == 0 {
		return nil
	}
	binHz := float64(Rate) / keyFFT
	out := make([]float64, 12)
	for _, m := range mags {
		for k := int(65 / binHz); k <= int(1000/binHz) && k < len(m); k++ {
			f := float64(k) * binHz
			pc := int(math.Round(12*math.Log2(f/440))+69) % 12
			if pc < 0 {
				pc += 12
			}
			out[pc] += m[k]
		}
	}
	return out
}

func pearson(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var num, da, db float64
	for i := range a {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

// keyOf returns the best of the 24 keys ("D minor") and a confidence: how far
// the best correlation is from the second one.
func keyOf(samples []float32) (string, float64) {
	c := chroma(samples)
	if c == nil {
		return "", 0
	}
	bestR, secondR := -2.0, -2.0
	best := ""
	for tonic := 0; tonic < 12; tonic++ {
		for mode, prof := range [][]float64{majorProfile, minorProfile} {
			rot := make([]float64, 12)
			for i := 0; i < 12; i++ {
				rot[(i+tonic)%12] = prof[i]
			}
			r := pearson(c, rot)
			name := noteNames[tonic] + " major"
			if mode == 1 {
				name = noteNames[tonic] + " minor"
			}
			if r > bestR {
				secondR, bestR, best = bestR, r, name
			} else if r > secondR {
				secondR = r
			}
		}
	}
	return best, math.Min(1, math.Max(0, (bestR-secondR)*4))
}
