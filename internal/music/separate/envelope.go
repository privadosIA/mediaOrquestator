package separate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
)

// The settings of the envelope are constants: the same stem gives the same envelope.
const (
	// Hop is the length of a frame of the envelope, in seconds.
	Hop = 0.02
	// smoothFrames is the smoothing of the level, 100 ms.
	smoothFrames = 5
	// noiseTimes is how many times the noise floor (the 10th percentile of the levels) a frame has to rise above.
	noiseTimes = 4.0
	// floorDB is the level, in dBFS, under which a frame is never voice.
	floorDB = -45.0
	// minVoiceFrames is the shortest run of voice kept, 60 ms; maxGapFrames the longest gap bridged, 150 ms.
	minVoiceFrames = 3
	maxGapFrames   = 7
)

// Env is where there is a voice: a frame of Hop seconds is voiced or not.
type Env struct {
	Hop    float64
	Voiced []bool
}

// EnvelopeOf makes the envelope of samples (mono) at rate Hz: the level (RMS) of every 20 ms smoothed over 100 ms; a frame is
// voiced when its level is above max(4 times the noise floor, -45 dBFS); runs of voice under 60 ms are dropped and gaps under
// 150 ms are bridged.
func EnvelopeOf(samples []float32, rate int) Env {
	frame := int(math.Round(Hop * float64(rate)))
	if frame < 1 || len(samples) < frame {
		return Env{Hop: Hop}
	}
	n := len(samples) / frame
	level := make([]float64, n)
	for i := 0; i < n; i++ {
		var sum float64
		for _, v := range samples[i*frame : (i+1)*frame] {
			sum += float64(v) * float64(v)
		}
		level[i] = math.Sqrt(sum / float64(frame))
	}
	smooth := make([]float64, n)
	half := smoothFrames / 2
	for i := range smooth {
		var sum float64
		count := 0
		for k := i - half; k <= i+half; k++ {
			if k >= 0 && k < n {
				sum += level[k]
				count++
			}
		}
		smooth[i] = sum / float64(count)
	}
	sorted := append([]float64(nil), smooth...)
	sort.Float64s(sorted)
	noise := sorted[len(sorted)/10]
	threshold := math.Max(noiseTimes*noise, math.Pow(10, floorDB/20))
	voiced := make([]bool, n)
	for i, v := range smooth {
		voiced[i] = v > threshold
	}
	// bridge the short gaps, then drop the short runs
	for i := 0; i < n; {
		if voiced[i] {
			i++
			continue
		}
		j := i
		for j < n && !voiced[j] {
			j++
		}
		if i > 0 && j < n && j-i <= maxGapFrames {
			for k := i; k < j; k++ {
				voiced[k] = true
			}
		}
		i = j
	}
	for i := 0; i < n; {
		if !voiced[i] {
			i++
			continue
		}
		j := i
		for j < n && voiced[j] {
			j++
		}
		if j-i < minVoiceFrames {
			for k := i; k < j; k++ {
				voiced[k] = false
			}
		}
		i = j
	}
	return Env{Hop: Hop, Voiced: voiced}
}

// EnvelopeFile reads a mono 16-bit PCM wav (the 16 kHz stem Whisper reads) and makes its envelope.
func EnvelopeFile(path string) (Env, error) {
	samples, rate, err := readWAV(path)
	if err != nil {
		return Env{}, err
	}
	return EnvelopeOf(samples, rate), nil
}

// readWAV reads a mono 16-bit PCM wav.
func readWAV(path string) ([]float32, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, 0, errors.New("not a wav file")
	}
	rate, channels, bits := 0, 0, 0
	for p := 12; p+8 <= len(raw); {
		id, size := string(raw[p:p+4]), int(binary.LittleEndian.Uint32(raw[p+4:p+8]))
		body := p + 8
		switch id {
		case "fmt ":
			if body+16 > len(raw) || binary.LittleEndian.Uint16(raw[body:]) != 1 {
				return nil, 0, errors.New("the wav is not PCM")
			}
			channels = int(binary.LittleEndian.Uint16(raw[body+2:]))
			rate = int(binary.LittleEndian.Uint32(raw[body+4:]))
			bits = int(binary.LittleEndian.Uint16(raw[body+14:]))
		case "data":
			if channels != 1 || bits != 16 || rate <= 0 {
				return nil, 0, fmt.Errorf("the wav must be mono 16-bit PCM (it has %d channels of %d bits)", channels, bits)
			}
			end := body + size
			if end > len(raw) || size < 0 {
				end = len(raw)
			}
			samples := make([]float32, (end-body)/2)
			for i := range samples {
				samples[i] = float32(int16(binary.LittleEndian.Uint16(raw[body+i*2:]))) / 32768
			}
			return samples, rate, nil
		}
		p = body + size + size%2
	}
	return nil, 0, errors.New("the wav has no data")
}

// VoicedShare is the share of the song, from 0 to 1, that has a voice.
func (e Env) VoicedShare() float64 {
	if len(e.Voiced) == 0 {
		return 0
	}
	n := 0
	for _, v := range e.Voiced {
		if v {
			n++
		}
	}
	return float64(n) / float64(len(e.Voiced))
}

// Onsets are the times (seconds) where the voice starts: a frame that is voiced after one that is not.
func (e Env) Onsets() []float64 {
	var out []float64
	for i, v := range e.Voiced {
		if v && (i == 0 || !e.Voiced[i-1]) {
			out = append(out, float64(i)*e.Hop)
		}
	}
	return out
}

// Share is the share, from 0 to 1, of [start, end) seconds that has a voice. A span with no frames (outside the song, or
// empty) is 0.
func (e Env) Share(start, end float64) float64 {
	if e.Hop <= 0 || end <= start {
		return 0
	}
	from, to := int(math.Floor(start/e.Hop)), int(math.Ceil(end/e.Hop))
	if from < 0 {
		from = 0
	}
	if to > len(e.Voiced) {
		to = len(e.Voiced)
	}
	if to <= from {
		return 0
	}
	n := 0
	for _, v := range e.Voiced[from:to] {
		if v {
			n++
		}
	}
	return float64(n) / float64(to-from)
}

// AnalyseFile reads a mono 16-bit PCM wav (the 16 kHz stem Whisper reads) and gives its envelope and its attacks.
func AnalyseFile(path string) (Env, []float64, error) {
	samples, rate, err := readWAV(path)
	if err != nil {
		return Env{}, nil, err
	}
	env := EnvelopeOf(samples, rate)
	return env, Attacks(samples, rate, env), nil
}
