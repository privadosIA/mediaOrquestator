package separate

import (
	"math"
	"math/cmplx"
)

// fftSize is the length of the frames the attack detector looks at: 1024 samples (64 ms at 16 kHz).
const fftSize = 1024

// twiddles are the roots of unity the transform multiplies by, computed once: twiddles[k] = e^(-2 pi i k / fftSize).
var twiddles = func() []complex128 {
	w := make([]complex128, fftSize/2)
	for k := range w {
		w[k] = cmplx.Exp(complex(0, -2*math.Pi*float64(k)/fftSize))
	}
	return w
}()

// bitReversed is the bit-reversal permutation of the indexes of a frame.
var bitReversed = func() []int {
	n, bits := fftSize, 0
	for 1<<bits < n {
		bits++
	}
	r := make([]int, n)
	for i := range r {
		x, y := i, 0
		for b := 0; b < bits; b++ {
			y = y<<1 | x&1
			x >>= 1
		}
		r[i] = y
	}
	return r
}()

// fft transforms x (of length fftSize) in place: an iterative radix-2 Cooley-Tukey transform. It is deterministic: the same
// input gives the same output.
func fft(x []complex128) {
	n := len(x)
	for i, j := range bitReversed {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half, step := size/2, n/size
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				t := twiddles[k*step] * x[start+k+half]
				u := x[start+k]
				x[start+k], x[start+k+half] = u+t, u-t
			}
		}
	}
}
