package separate

import (
	"math"
	"math/cmplx"
	"sort"
)

// The settings of the attack detector are constants: the same stem gives the same attacks.
const (
	// attackHop is the step between two analysis frames, in seconds (10 ms).
	attackHop = 0.01
	// attackLowHz and attackHighHz bound the part of the spectrum that is looked at: where a voice attacks, not the rumble under it
	// nor the hiss over it.
	attackLowHz, attackHighHz = 150.0, 5000.0
	// attackCompress is the gain before the logarithm that compresses the magnitudes (a quiet syllable counts as much as a loud one).
	attackCompress = 100.0
	// attackMedianSpan is half the window, in seconds, of the local median the flux has to rise above.
	attackMedianSpan = 0.5
	// attackPeakSpan is how far, in seconds, a peak must be the maximum of the flux on either side.
	attackPeakSpan = 0.04
	// attackVoicedAfter is how long after a peak the envelope must still hear the voice, in seconds.
	attackVoicedAfter = 0.02
	// MinAttackGap is the least distance, in seconds, between two attacks: of two closer ones the earlier stays.
	MinAttackGap = 0.08
)

// Attacks finds where the voice attacks (where a syllable starts) in a mono stem of rate Hz: the positive change of the spectrum from
// one frame of 1024 samples to the next (spectral flux, in the band of the voice, with the magnitudes compressed by a logarithm),
// a peak of it when it is above the local median plus the mean of the whole stem, is the highest within 40 ms either side and has
// voice 20 ms after it, and attacks closer than MinAttackGap are merged. About one per syllable. The times are in seconds, in
// order. env is the envelope of the same stem (an empty one asks nothing of the voice). The same input gives the same attacks.
func Attacks(samples []float32, rate int, env Env) []float64 {
	hop := int(math.Round(attackHop * float64(rate)))
	if hop < 1 || len(samples) < fftSize+hop {
		return []float64{}
	}
	frames := (len(samples)-fftSize)/hop + 1
	lo := int(math.Ceil(attackLowHz * fftSize / float64(rate)))
	hi := int(math.Floor(attackHighHz * fftSize / float64(rate)))
	if hi >= fftSize/2 {
		hi = fftSize/2 - 1
	}
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		return []float64{}
	}
	hann := make([]float64, fftSize)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize))
	}
	flux := make([]float64, frames)
	prev := make([]float64, hi-lo+1)
	cur := make([]float64, hi-lo+1)
	buf := make([]complex128, fftSize)
	for t := 0; t < frames; t++ {
		start := t * hop
		for i := 0; i < fftSize; i++ {
			buf[i] = complex(float64(samples[start+i])*hann[i], 0)
		}
		fft(buf)
		var sum float64
		for k := lo; k <= hi; k++ {
			cur[k-lo] = math.Log1p(attackCompress * cmplx.Abs(buf[k]) / (fftSize / 2))
			if t > 0 {
				if d := cur[k-lo] - prev[k-lo]; d > 0 {
					sum += d
				}
			}
		}
		flux[t] = sum
		prev, cur = cur, prev
	}
	var mean float64
	for _, f := range flux {
		mean += f
	}
	mean /= float64(frames)
	span := int(math.Round(attackMedianSpan / attackHop))
	peak := int(math.Round(attackPeakSpan / attackHop))
	window := make([]float64, 0, 2*span+1)
	var found []float64
	for t := 1; t < frames-1; t++ {
		window = window[:0]
		for u := max(0, t-span); u <= min(frames-1, t+span); u++ {
			window = append(window, flux[u])
		}
		sort.Float64s(window)
		if flux[t] <= window[len(window)/2]+mean {
			continue
		}
		top := true
		for u := max(0, t-peak); u <= min(frames-1, t+peak) && top; u++ {
			if u < t && flux[u] >= flux[t] || u > t && flux[u] > flux[t] {
				top = false
			}
		}
		if !top {
			continue
		}
		at := float64(t*hop+fftSize/2) / float64(rate) // the middle of the frame
		if len(env.Voiced) > 0 && env.Hop > 0 {
			f := int((at + attackVoicedAfter) / env.Hop)
			if f >= len(env.Voiced) || !env.Voiced[f] {
				continue
			}
		}
		if n := len(found); n > 0 && at-found[n-1] < MinAttackGap {
			continue
		}
		found = append(found, math.Round(at*1000)/1000)
	}
	if found == nil {
		found = []float64{}
	}
	return found
}
