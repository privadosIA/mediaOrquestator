// Package beatgrid finds where the beats of a song fall from its audio samples and its tempo: [Phase] gives the
// offset of the pulse, [Track] follows the beats through the song when the tempo drifts. It reads only the start of
// the audio and has no dependency on the rest of the program, so the beat-synchronised cuts and the music-motion
// timeline both use it.
package beatgrid

import (
	"math"
	"sort"
)

const (
	beatHop        = 0.010 // seconds between energy frames
	beatWindow     = 0.020 // seconds each energy frame covers
	beatMaxSecs    = 30.0  // only the start of the song is read: the tempo drifts over a whole song
	beatTempoRange = 0.03  // tempos tried: the stated one, plus or minus this fraction
	beatTempoStep  = 0.005
	beatMinRatio   = 1.5 // how much better than average the best phase must score to be believed
	beatMinFrames  = 200 // shorter audio than 2 s says nothing about its pulse
)

// Phase estimates where the beats of a song fall, given its tempo: the
// second, from 0 up to one beat, at which the pulse is aligned (beats are at
// phase + k * 60/bpm). It slides a comb of the beat period over the onsets
// (the rises in loudness) of the first minute and keeps the offset where
// the most onset energy lands on it. It returns false when the audio is too
// short, silent, or has no pulse clearer than chance, so the caller can fall
// back on something else instead of trusting a guess.
func Phase(samples []float32, rate int, bpm float64) (float64, bool) {
	if bpm < 20 || bpm > 400 || rate <= 0 {
		return 0, false
	}
	hop, win := int(beatHop*float64(rate)), int(beatWindow*float64(rate))
	if hop < 1 || win < 1 {
		return 0, false
	}
	limit := len(samples)
	if max := int(beatMaxSecs * float64(rate)); limit > max {
		limit = max
	}
	n := (limit - win) / hop
	if n < beatMinFrames {
		return 0, false
	}
	energy := make([]float64, n)
	for i := range energy {
		var sum float64
		for _, v := range samples[i*hop : i*hop+win] {
			sum += float64(v) * float64(v)
		}
		energy[i] = math.Sqrt(sum / float64(win))
	}
	onset := make([]float64, n)
	var total float64
	for i := 1; i < n; i++ {
		if d := energy[i] - energy[i-1]; d > 0 {
			onset[i] = d
			total += d
		}
	}
	if total == 0 {
		return 0, false
	}
	// The engine's tempo is approximate, and a comb of the wrong period drifts off the pulse
	// within a few beats, so a few tempos around the stated one are tried as well. What is
	// reported is where the beats fall at the start, which is where a clip is placed by.
	bestPhase, bestRatio := 0, 0.0
	for pct := -beatTempoRange; pct <= beatTempoRange+1e-9; pct += beatTempoStep {
		period := 60 / (bpm * (1 + pct)) / beatHop // frames per beat
		steps := int(math.Round(period))
		if steps < 2 {
			continue
		}
		var best, sum float64
		at := 0
		for p := 0; p < steps; p++ {
			var sc float64
			for k := 0; ; k++ {
				frame := int(math.Round(float64(p) + float64(k)*period))
				if frame >= n-1 {
					break
				}
				// The strongest onset within a frame either side: the comb is not exact.
				sc += strongest(onset, frame)
			}
			sum += sc
			if sc > best {
				best, at = sc, p
			}
		}
		if mean := sum / float64(steps); mean > 0 && best/mean > bestRatio {
			bestRatio, bestPhase = best/mean, at
		}
	}
	if bestRatio < beatMinRatio {
		return 0, false
	}
	// A frame is stamped with its centre: the onset shows once the window has taken in the attack.
	return math.Round((float64(bestPhase)*beatHop+beatWindow/2)*1000) / 1000, true
}

// strongest is the largest value within one frame either side of at.
func strongest(v []float64, at int) float64 {
	m := v[at]
	if at > 0 && v[at-1] > m {
		m = v[at-1]
	}
	if at+1 < len(v) && v[at+1] > m {
		m = v[at+1]
	}
	return m
}

// Tracked is the result of following the pulse of a whole song.
type Tracked struct {
	// BPM is the tempo the pulse really has (the stated one corrected by what the song does).
	BPM float64
	// Beats are the time of every beat, following the drift of the pulse.
	Beats []float64
}

// onsetCurve is the rise in loudness at every hop of the audio: where the pulse can be heard.
func onsetCurve(samples []float32, rate int) ([]float64, bool) {
	hop, win := int(beatHop*float64(rate)), int(beatWindow*float64(rate))
	if hop < 1 || win < 1 || len(samples) < win+hop*beatMinFrames {
		return nil, false
	}
	n := (len(samples) - win) / hop
	energy := make([]float64, n)
	for i := range energy {
		var sum float64
		for _, v := range samples[i*hop : i*hop+win] {
			sum += float64(v) * float64(v)
		}
		energy[i] = math.Sqrt(sum / float64(win))
	}
	onset := make([]float64, n)
	var total float64
	for i := 1; i < n; i++ {
		if d := energy[i] - energy[i-1]; d > 0 {
			onset[i] = d
			total += d
		}
	}
	return onset, total > 0
}

const (
	trackTempoRange = 0.05 // tempos tried: the stated one, plus or minus this fraction
	trackTempoStep  = 0.001
	// trackTightness is how hard the tracker holds to the fitted tempo: a beat that comes at twice the expected
	// interval costs this much against the onset energy it finds (the envelope is scaled to a deviation of 1).
	trackTightness = 120.0
)

// Track follows the pulse of a whole song at about the stated tempo. A song made by a model does not keep an exact
// tempo, and the tempo it was asked for is not the one it has, so a fixed grid drifts off the pulse within a minute.
// The tempo is fitted over the whole song first; then the beats are placed by dynamic programming (the beat tracker
// of Ellis, 2007): the sequence of beats that gathers the most onset energy while keeping each interval close to the
// fitted period, so the grid bends with the song and does not jump for one odd onset. It returns false when the audio
// has no pulse clearer than chance, so the caller can fall back on a plain grid.
func Track(samples []float32, rate int, bpm float64) (Tracked, bool) {
	if bpm < 20 || bpm > 400 || rate <= 0 {
		return Tracked{}, false
	}
	onset, ok := onsetCurve(samples, rate)
	if !ok {
		return Tracked{}, false
	}
	n := len(onset)
	// 1. The tempo that fits the whole song.
	bestRatio, bestPct := 0.0, 0.0
	for pct := -trackTempoRange; pct <= trackTempoRange+1e-9; pct += trackTempoStep {
		period := 60 / (bpm * (1 + pct)) / beatHop // frames per beat
		steps := int(math.Round(period))
		if steps < 2 {
			continue
		}
		var best, sum float64
		for p := 0; p < steps; p++ {
			var sc float64
			for k := 0; ; k++ {
				frame := int(math.Round(float64(p) + float64(k)*period))
				if frame >= n-1 {
					break
				}
				sc += strongest(onset, frame)
			}
			sum += sc
			if sc > best {
				best = sc
			}
		}
		if mean := sum / float64(steps); mean > 0 && best/mean > bestRatio {
			bestRatio, bestPct = best/mean, pct
		}
	}
	if bestRatio < beatMinRatio {
		return Tracked{}, false
	}
	period := 60 / (bpm * (1 + bestPct)) / beatHop // frames per beat
	// 2. The best chain of beats: score(t) = onset(t) + max over the previous beat u of score(u) - tightness*ln(gap/period)^2.
	var mean, sq float64
	for _, v := range onset {
		mean += v
	}
	mean /= float64(n)
	for _, v := range onset {
		sq += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(sq / float64(n))
	if sd == 0 {
		return Tracked{}, false
	}
	score := make([]float64, n)
	prev := make([]int, n)
	lo, hi := int(math.Round(period/2)), int(math.Round(period*2))
	if lo < 1 {
		lo = 1
	}
	for t := 0; t < n; t++ {
		o := onset[t] / sd
		best, from := 0.0, -1 // a chain may start here
		for g := lo; g <= hi && t-g >= 0; g++ {
			d := math.Log(float64(g) / period)
			if c := score[t-g] - trackTightness*d*d; c > best {
				best, from = c, t-g
			}
		}
		score[t], prev[t] = o+best, from
	}
	// The chain that ends in the last stretch of the song and has the most score.
	end, bestScore := -1, math.Inf(-1)
	for t := n - int(math.Round(period)); t < n; t++ {
		if t >= 0 && score[t] > bestScore {
			end, bestScore = t, score[t]
		}
	}
	if end < 0 {
		return Tracked{}, false
	}
	var frames []int
	for t := end; t >= 0; t = prev[t] {
		frames = append(frames, t)
		if prev[t] < 0 {
			break
		}
	}
	for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
		frames[i], frames[j] = frames[j], frames[i]
	}
	if len(frames) < 4 {
		return Tracked{}, false
	}
	beats := make([]float64, len(frames))
	gaps := make([]float64, 0, len(frames)-1)
	for i, f := range frames {
		beats[i] = math.Round((float64(f)*beatHop+beatWindow/2)*1000) / 1000
		if i > 0 {
			gaps = append(gaps, beats[i]-beats[i-1])
		}
	}
	sort.Float64s(gaps)
	med := gaps[len(gaps)/2]
	return Tracked{BPM: math.Round(60/med*100) / 100, Beats: beats}, true
}
