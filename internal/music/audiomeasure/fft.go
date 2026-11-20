package audiomeasure

import (
	"math"
	"math/cmplx"
)

// fft is an in-place radix-2 transform; len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Rect(1, -2*math.Pi/float64(size))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := 0; k < size/2; k++ {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}

// magnitudes returns the magnitude spectrum (n/2+1 bins) of every frame of
// size n taken every hop samples, windowed with a Hann window.
func magnitudes(samples []float32, n, hop int) [][]float64 {
	if len(samples) < n {
		return nil
	}
	win := make([]float64, n)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	frames := (len(samples)-n)/hop + 1
	out := make([][]float64, frames)
	buf := make([]complex128, n)
	for f := 0; f < frames; f++ {
		off := f * hop
		for i := 0; i < n; i++ {
			buf[i] = complex(float64(samples[off+i])*win[i], 0)
		}
		fft(buf)
		m := make([]float64, n/2+1)
		for i := range m {
			m[i] = cmplx.Abs(buf[i])
		}
		out[f] = m
	}
	return out
}
