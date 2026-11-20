package audiomeasure

import (
	"math"
	"sort"
)

// Sound describes how a piece of music sounds, with figures any estimator can measure and no model: how
// loud and how dynamic it is, how bright and how bass-heavy, how noisy (distorted or breathy) and how
// busy its rhythm is. They stand beside the tempo and the key to tell a slow dark piano piece from a
// bright dense dance track when there is no text to go on. They are estimates over the whole clip.
type Sound struct {
	// LoudnessDB is the mean level of the frames that carry sound, in dB below full scale (about -30 quiet, -12 loud).
	LoudnessDB float64
	// DynamicsDB is the spread between the loud and the quiet frames (low in compressed dance music).
	DynamicsDB float64
	// CentroidHz is the average centre of the spectrum: low is dark, high is bright.
	CentroidHz float64
	// BassShare and HighShare are the shares of the energy below 250 Hz and above 4 kHz.
	BassShare float64
	HighShare float64
	// Flatness is how noise-like the spectrum is (0 tonal, 1 noise).
	Flatness float64
	// OnsetsPerSecond is how many note or beat attacks there are per second.
	OnsetsPerSecond float64
}

const (
	soundFFT, soundHop = 2048, 1024
	// silenceDB is the level under which a frame is silence and does not count.
	silenceDB = -55.0
)

// Describe measures the sound of mono samples at Rate. The second result is false when there was too little
// sound to say anything.
func Describe(samples []float32) (Sound, bool) {
	mags := magnitudes(samples, soundFFT, soundHop)
	if len(mags) < 8 {
		return Sound{}, false
	}
	binHz := float64(Rate) / soundFFT
	bassTop, highFrom := int(250/binHz), int(4000/binHz)
	var s Sound
	var levels []float64
	var n int
	for _, m := range mags {
		var energy, bass, high, weighted, logSum float64
		for k := 1; k < len(m); k++ {
			e := m[k] * m[k]
			energy += e
			weighted += e * float64(k) * binHz
			if k <= bassTop {
				bass += e
			}
			if k >= highFrom {
				high += e
			}
			logSum += math.Log(e + 1e-12)
		}
		if energy <= 0 {
			continue
		}
		level := 10 * math.Log10(energy/float64(len(m)*len(m))+1e-12)
		if level < silenceDB {
			continue
		}
		levels = append(levels, level)
		n++
		s.CentroidHz += weighted / energy
		s.BassShare += bass / energy
		s.HighShare += high / energy
		// Spectral flatness: geometric over arithmetic mean of the power.
		s.Flatness += math.Exp(logSum/float64(len(m)-1)) / (energy / float64(len(m)-1))
	}
	if n < 8 {
		return Sound{}, false
	}
	f := float64(n)
	s.CentroidHz /= f
	s.BassShare /= f
	s.HighShare /= f
	s.Flatness /= f
	sort.Float64s(levels)
	var sum float64
	for _, l := range levels {
		sum += l
	}
	s.LoudnessDB = sum / f
	s.DynamicsDB = levels[int(0.95*float64(n-1))] - levels[int(0.10*float64(n-1))]

	env, fps := onsetEnvelope(samples)
	if len(env) > 0 {
		s.OnsetsPerSecond = float64(countPeaks(env)) / (float64(len(env)) / fps)
	}
	return s, true
}

// countPeaks counts the clear peaks of an onset envelope: local maxima above half of the mean of its
// positive values, at least 80 ms apart.
func countPeaks(env []float64) int {
	var sum float64
	var pos int
	for _, v := range env {
		if v > 0 {
			sum += v
			pos++
		}
	}
	if pos == 0 {
		return 0
	}
	thr := 0.5 * sum / float64(pos)
	hop := float64(tempoHop)
	gap := int(0.08 * float64(Rate) / hop)
	last, peaks := -gap, 0
	for i := 1; i < len(env)-1; i++ {
		if env[i] > thr && env[i] >= env[i-1] && env[i] > env[i+1] && i-last >= gap {
			peaks++
			last = i
		}
	}
	return peaks
}

// Words says in a few plain words what the figures of a Sound mean, for the assistant (and a person) to read;
// only what is clear is said. The thresholds come from clips of generated songs of known styles.
func (s Sound) Words() []string {
	var w []string
	switch {
	case s.CentroidHz < 450:
		w = append(w, "dark, low-pitched timbre")
	case s.CentroidHz > 900:
		w = append(w, "bright timbre")
	default:
		w = append(w, "warm mid-range timbre")
	}
	if s.BassShare > 0.55 {
		w = append(w, "bass-heavy")
	}
	if s.HighShare > 0.06 {
		w = append(w, "lots of high-frequency content (sharp synths, cymbals or distortion)")
	}
	switch {
	case s.Flatness > 0.05:
		w = append(w, "noisy, electronic or distorted texture")
	case s.Flatness < 0.005:
		w = append(w, "clean, tonal texture")
	}
	switch {
	case s.DynamicsDB < 10:
		w = append(w, "very steady level (compressed or sustained)")
	case s.DynamicsDB > 16:
		w = append(w, "wide dynamics (acoustic or live feel)")
	}
	switch {
	case s.OnsetsPerSecond > 5.5:
		w = append(w, "busy, dense rhythm")
	case s.OnsetsPerSecond < 2.5:
		w = append(w, "sparse rhythm")
	}
	switch {
	case s.LoudnessDB > -16:
		w = append(w, "loud")
	case s.LoudnessDB < -24:
		w = append(w, "quiet")
	}
	return w
}
