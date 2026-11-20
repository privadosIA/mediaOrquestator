package videoclip

import "math"

// Alignment of the cuts between sections to the song itself. The time each section deserves is first
// estimated from the text it sings; the song usually changes where the music changes (the chorus comes in
// louder, a break drops), so each boundary is moved to the nearest clear change of loudness.

const (
	// EnvelopeHop is the seconds each envelope value covers.
	EnvelopeHop = 0.25
	// contrastSeconds is how much audio before and after a point is compared.
	contrastSeconds = 3.0
	// maxMoveSeconds caps how far a boundary may be moved.
	maxMoveSeconds = 8.0
	// minChange is the smallest relative change of loudness that counts as a change of section.
	minChange = 0.25
	// distancePenalty prefers the change that is closest to the estimate when two are alike.
	distancePenalty = 0.3
)

// RefineBoundaries moves each boundary (the start of a section after the first) to the clearest change of
// loudness near it. env is the loudness (RMS) of the song every EnvelopeHop seconds. A boundary is only moved
// when the change there is at least minChange of the average loudness around it, never further than
// maxMoveSeconds or a third of the shorter section next to it, and the order and a minimum length of a section
// are kept. It returns the new boundaries and how many moved.
func RefineBoundaries(starts []float64, env []float64, total float64) ([]float64, int) {
	out := append([]float64(nil), starts...)
	if len(env) < 8 || len(starts) == 0 {
		return out, 0
	}
	prefix := make([]float64, len(env)+1)
	for i, v := range env {
		prefix[i+1] = prefix[i] + v
	}
	mean := func(a, b float64) float64 { // average loudness between two seconds
		i, j := clampIdx(int(a/EnvelopeHop), len(env)), clampIdx(int(b/EnvelopeHop), len(env))
		if j <= i {
			return 0
		}
		return (prefix[j] - prefix[i]) / float64(j-i)
	}
	moved := 0
	for k, b := range starts {
		prev, next := 0.0, total
		if k > 0 {
			prev = out[k-1]
		}
		if k+1 < len(starts) {
			next = starts[k+1]
		}
		reach := math.Min(maxMoveSeconds, math.Min(b-prev, next-b)/3)
		if reach < EnvelopeHop*2 {
			continue
		}
		best, bestScore := b, 0.0
		for t := b - reach; t <= b+reach; t += EnvelopeHop {
			before, after := mean(t-contrastSeconds, t), mean(t, t+contrastSeconds)
			level := (before + after) / 2
			if level <= 0 {
				continue
			}
			change := math.Abs(after-before) / level
			if change < minChange {
				continue
			}
			score := change * (1 - distancePenalty*math.Abs(t-b)/reach)
			if score > bestScore {
				best, bestScore = t, score
			}
		}
		if best != b {
			out[k] = math.Round(best/EnvelopeHop) * EnvelopeHop
			moved++
		}
	}
	return out, moved
}

func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i > n {
		return n
	}
	return i
}

// Envelope reduces mono samples at the given rate to the loudness (RMS) of every EnvelopeHop seconds.
func Envelope(samples []float32, rate int) []float64 {
	per := int(EnvelopeHop * float64(rate))
	if per < 1 {
		return nil
	}
	out := make([]float64, 0, len(samples)/per)
	for i := 0; i+per <= len(samples); i += per {
		var sum float64
		for _, v := range samples[i : i+per] {
			sum += float64(v) * float64(v)
		}
		out = append(out, math.Sqrt(sum/float64(per)))
	}
	return out
}
