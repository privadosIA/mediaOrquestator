package separate

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

// dft is the transform by its definition, the slow way, to compare the fast one with.
func dft(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := 0; k < n; k++ {
		var s complex128
		for t := 0; t < n; t++ {
			s += x[t] * cmplx.Exp(complex(0, -2*math.Pi*float64(k*t)/float64(n)))
		}
		out[k] = s
	}
	return out
}

func TestFFT_MatchesTheDirectTransformOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	x := make([]complex128, fftSize)
	for i := range x {
		x[i] = complex(rng.Float64()*2-1, rng.Float64()*2-1)
	}
	want := dft(x)
	got := append([]complex128(nil), x...)
	fft(got)
	var worst float64
	for i := range got {
		if d := cmplx.Abs(got[i] - want[i]); d > worst {
			worst = d
		}
	}
	if worst > 1e-4 {
		t.Fatalf("the largest error is %g, want under 1e-4", worst)
	}
}

func TestFFT_APureToneLandsInItsBin(t *testing.T) {
	x := make([]complex128, fftSize)
	const bin = 37
	for i := range x {
		x[i] = complex(math.Cos(2*math.Pi*bin*float64(i)/fftSize), 0)
	}
	fft(x)
	best, at := 0.0, 0
	for k := 1; k < fftSize/2; k++ {
		if m := cmplx.Abs(x[k]); m > best {
			best, at = m, k
		}
	}
	if at != bin || math.Abs(best-fftSize/2) > 1e-6 {
		t.Fatalf("peak at bin %d with magnitude %g, want bin %d and %d", at, best, bin, fftSize/2)
	}
}

func TestFFT_IsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	x := make([]complex128, fftSize)
	for i := range x {
		x[i] = complex(rng.Float64(), 0)
	}
	a, b := append([]complex128(nil), x...), append([]complex128(nil), x...)
	fft(a)
	fft(b)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("differs at %d", i)
		}
	}
}
