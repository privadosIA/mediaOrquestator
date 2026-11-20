package audiomeasure

import (
	"math"
	"strings"
	"testing"
)

// tone is a pure sine of the given frequency and level, seconds long.
func tone(hz, amp float64, seconds float64) []float32 {
	n := int(seconds * Rate)
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*hz*float64(i)/Rate))
	}
	return out
}

func TestDescribe_ALowToneIsDarkAndABrightOneIsBright(t *testing.T) {
	low, ok := Describe(tone(110, 0.3, 6))
	if !ok {
		t.Fatal("a tone has sound")
	}
	high, ok := Describe(tone(3000, 0.3, 6))
	if !ok {
		t.Fatal("a tone has sound")
	}
	if low.CentroidHz >= high.CentroidHz || low.BassShare < 0.9 {
		t.Errorf("low %+v / high %+v", low, high)
	}
	if got := strings.Join(low.Words(), " "); !strings.Contains(got, "dark") || !strings.Contains(got, "bass-heavy") || !strings.Contains(got, "clean, tonal") {
		t.Errorf("low words: %s", got)
	}
	if got := strings.Join(high.Words(), " "); !strings.Contains(got, "bright") {
		t.Errorf("high words: %s", got)
	}
}

func TestDescribe_SilenceAndShortClipsSayNothing(t *testing.T) {
	if _, ok := Describe(make([]float32, Rate*6)); ok {
		t.Error("silence has nothing to describe")
	}
	if _, ok := Describe(make([]float32, 100)); ok {
		t.Error("too short")
	}
}

func TestDescribe_NoiseIsFlatterThanATone(t *testing.T) {
	noise := make([]float32, Rate*6)
	seed := uint32(12345)
	for i := range noise {
		seed = seed*1664525 + 1013904223
		noise[i] = (float32(seed>>8)/float32(1<<24) - 0.5) * 0.5
	}
	n, _ := Describe(noise)
	tn, _ := Describe(tone(440, 0.3, 6))
	if n.Flatness <= tn.Flatness*10 {
		t.Errorf("noise flatness %.4f vs tone %.4f", n.Flatness, tn.Flatness)
	}
}
