// Package audiomeasure estimates the tempo and the key of a piece of music
// from its samples. It uses no model and no program: the tempo comes from the
// autocorrelation of a spectral-flux onset envelope (weighted towards 120 bpm,
// the usual prior of the method of Ellis, 2007) and the key from a chroma
// profile compared with the Krumhansl-Kessler major and minor profiles. Both
// are estimates: the tempo is often found at half or double its value and the
// key at its relative major or minor, so the answer carries alternatives and a
// confidence, and says nothing (nil) when there is no clear pulse or key.
package audiomeasure

import (
	"math"
	"sort"
)

// Rate is the sample rate Measure expects (mono).
const Rate = 22050

const (
	minBPM, maxBPM = 60.0, 200.0

	tempoFFT, tempoHop = 2048, 256
	keyFFT, keyHop     = 8192, 4096

	// minPulse and minKey are the confidences below which the answer is nil.
	minPulse = 0.08
	minKey   = 0.05
)

// Result is what Measure found. BPM and Key are nil when there was no clear
// pulse or key.
type Result struct {
	Duration       float64
	BPM            *float64
	BPMAlternative []float64
	BPMConfidence  float64
	Key            *string
	KeyConfidence  float64
}

// Measure estimates the tempo and key of mono samples at Rate.
func Measure(samples []float32) Result {
	res := Result{Duration: float64(len(samples)) / Rate}
	bpm, alts, conf := tempo(samples)
	res.BPMConfidence = conf
	if bpm > 0 && conf >= minPulse {
		res.BPM = &bpm
		res.BPMAlternative = alts
	}
	key, kconf := keyOf(samples)
	res.KeyConfidence = kconf
	if key != "" && kconf >= minKey {
		res.Key = &key
	}
	return res
}

// onsetEnvelope is the positive spectral flux of the log-compressed spectrum,
// with its local mean removed, one value per hop.
func onsetEnvelope(samples []float32) ([]float64, float64) {
	mags := magnitudes(samples, tempoFFT, tempoHop)
	if len(mags) < 8 {
		return nil, 0
	}
	// Up to ~8 kHz: the high end only adds noise to the flux.
	binHz := float64(Rate) / tempoFFT
	top := int(8000 / binHz)
	env := make([]float64, len(mags))
	prev := make([]float64, top)
	for f, m := range mags {
		var flux float64
		for k := 0; k < top; k++ {
			v := math.Log1p(1000 * m[k])
			if d := v - prev[k]; d > 0 {
				flux += d
			}
			prev[k] = v
		}
		env[f] = flux
	}
	fps := float64(Rate) / tempoHop
	// Remove a moving average of about half a second and keep what rises above it.
	w := int(fps / 2)
	out := make([]float64, len(env))
	for i := range env {
		lo, hi := i-w, i+w
		if lo < 0 {
			lo = 0
		}
		if hi >= len(env) {
			hi = len(env) - 1
		}
		var s float64
		for j := lo; j <= hi; j++ {
			s += env[j]
		}
		mean := s / float64(hi-lo+1)
		if v := env[i] - mean; v > 0 {
			out[i] = v
		}
	}
	return out, fps
}

func tempo(samples []float32) (bpm float64, alts []float64, conf float64) {
	env, fps := onsetEnvelope(samples)
	if len(env) == 0 {
		return 0, nil, 0
	}
	minLag := int(math.Floor(fps * 60 / maxBPM))
	maxLag := int(math.Ceil(fps * 60 / minBPM))
	if maxLag >= len(env)/2 {
		return 0, nil, 0
	}
	var zero float64
	for _, v := range env {
		zero += v * v
	}
	if zero == 0 {
		return 0, nil, 0
	}
	ac := make([]float64, maxLag+2)
	for lag := minLag - 1; lag <= maxLag+1; lag++ {
		if lag < 1 {
			continue
		}
		var s float64
		for i := 0; i+lag < len(env); i++ {
			s += env[i] * env[i+lag]
		}
		// Unbiased for the shrinking overlap.
		ac[lag] = s / zero * float64(len(env)) / float64(len(env)-lag)
	}
	best, bestScore := 0, 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		b := fps * 60 / float64(lag)
		// Log-Gaussian preference for 120 bpm, 1.4 octaves wide.
		w := math.Exp(-0.5 * math.Pow(math.Log2(b/120)/1.4, 2))
		if score := ac[lag] * w; score > bestScore && ac[lag] >= ac[lag-1] && ac[lag] >= ac[lag+1] {
			best, bestScore = lag, score
		}
	}
	if best == 0 {
		return 0, nil, 0
	}
	// Parabolic interpolation of the lag.
	l := float64(best)
	if a, b, c := ac[best-1], ac[best], ac[best+1]; a-2*b+c != 0 {
		l += 0.5 * (a - c) / (a - 2*b + c)
	}
	bpm = round1(fps * 60 / l)
	conf = math.Min(1, math.Max(0, ac[best]))
	for _, f := range []float64{0.5, 2} {
		if a := round1(bpm * f); a >= minBPM && a <= maxBPM {
			alts = append(alts, a)
		}
	}
	sort.Float64s(alts)
	return bpm, alts, conf
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
