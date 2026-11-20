package motionref

import (
	"fmt"
	"image"
	"math"
	"sort"
)

// Colours of a video: a deterministic k-means in Lab space over a grid of pixels of the pictures, with fixed
// settings, so the same pictures always give the same colours.
const (
	// DominantColours is how many colours are given.
	DominantColours = 5
	kmeansRounds    = 24
	gridW, gridH    = 48, 27
)

type lab [3]float64

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func linearToSRGB(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

var (
	white = [3]float64{0.95047, 1.0, 1.08883}
)

func labF(t float64) float64 {
	if t > 216.0/24389 {
		return math.Cbrt(t)
	}
	return (24389.0/27*t + 16) / 116
}

func labFInv(t float64) float64 {
	if t3 := t * t * t; t3 > 216.0/24389 {
		return t3
	}
	return (116*t - 16) / (24389.0 / 27)
}

// rgbToLab converts 8-bit sRGB to CIE Lab (D65).
func rgbToLab(r, g, b float64) lab {
	lr, lg, lb := srgbToLinear(r/255), srgbToLinear(g/255), srgbToLinear(b/255)
	x := 0.4124564*lr + 0.3575761*lg + 0.1804375*lb
	y := 0.2126729*lr + 0.7151522*lg + 0.0721750*lb
	z := 0.0193339*lr + 0.1191920*lg + 0.9503041*lb
	fx, fy, fz := labF(x/white[0]), labF(y/white[1]), labF(z/white[2])
	return lab{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// labToHex converts a Lab colour to #rrggbb (clamped to the sRGB gamut).
func labToHex(c lab) string {
	fy := (c[0] + 16) / 116
	fx, fz := fy+c[1]/500, fy-c[2]/200
	x, y, z := labFInv(fx)*white[0], labFInv(fy)*white[1], labFInv(fz)*white[2]
	lr := 3.2404542*x - 1.5371385*y - 0.4985314*z
	lg := -0.9692660*x + 1.8760108*y + 0.0415560*z
	lb := 0.0556434*x - 0.2040259*y + 1.0572252*z
	ch := func(v float64) int {
		v = math.Max(0, math.Min(1, linearToSRGB(math.Max(0, math.Min(1, v)))))
		return int(math.Round(v * 255))
	}
	return fmt.Sprintf("#%02x%02x%02x", ch(lr), ch(lg), ch(lb))
}

func dist2(a, b lab) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return d0*d0 + d1*d1 + d2*d2
}

// pixelsOf takes a gridW x gridH grid of pixels of a picture, in Lab.
func pixelsOf(img image.Image) []lab {
	b := img.Bounds()
	out := make([]lab, 0, gridW*gridH)
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*gridW)
			y := b.Min.Y + (2*gy+1)*b.Dy()/(2*gridH)
			r, g, bl, _ := img.At(x, y).RGBA()
			out = append(out, rgbToLab(float64(r>>8), float64(g>>8), float64(bl>>8)))
		}
	}
	return out
}

// dominant reduces pixels to at most k colours with their shares, the most common first.
func dominant(px []lab, k int) []Colour {
	if len(px) == 0 {
		return []Colour{}
	}
	// Farthest-point start: the first pixel, then the pixel farthest from those chosen (the first of the ties).
	centres := []lab{px[0]}
	for len(centres) < k {
		best, bi := -1.0, -1
		for i, p := range px {
			d := math.Inf(1)
			for _, c := range centres {
				d = math.Min(d, dist2(p, c))
			}
			if d > best {
				best, bi = d, i
			}
		}
		if best <= 0 {
			break // fewer different colours than k
		}
		centres = append(centres, px[bi])
	}
	counts := make([]int, len(centres))
	for round := 0; round < kmeansRounds; round++ {
		sums := make([]lab, len(centres))
		counts = make([]int, len(centres))
		for _, p := range px {
			bi, bd := 0, math.Inf(1)
			for i, c := range centres {
				if d := dist2(p, c); d < bd {
					bi, bd = i, d
				}
			}
			counts[bi]++
			for j := 0; j < 3; j++ {
				sums[bi][j] += p[j]
			}
		}
		for i := range centres {
			if counts[i] > 0 {
				for j := 0; j < 3; j++ {
					centres[i][j] = sums[i][j] / float64(counts[i])
				}
			}
		}
	}
	out := make([]Colour, 0, len(centres))
	for i, c := range centres {
		if counts[i] == 0 {
			continue
		}
		out = append(out, Colour{Hex: labToHex(c), Share: math.Round(float64(counts[i])/float64(len(px))*1000) / 1000})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Share != out[j].Share {
			return out[i].Share > out[j].Share
		}
		return out[i].Hex < out[j].Hex
	})
	return out
}

// coloursOf gives the dominant colours of all the pictures and of each third of them (they are in time order).
func coloursOf(takes []taken) ([]Colour, [][]Colour) {
	var all []lab
	per := make([][]lab, len(takes))
	for i, t := range takes {
		img, err := decode(t.path)
		if err != nil {
			continue
		}
		per[i] = pixelsOf(img)
		all = append(all, per[i]...)
	}
	thirds := make([][]Colour, 3)
	for k := 0; k < 3; k++ {
		var px []lab
		for i := k * len(per) / 3; i < (k+1)*len(per)/3; i++ {
			px = append(px, per[i]...)
		}
		thirds[k] = dominant(px, DominantColours)
	}
	return dominant(all, DominantColours), thirds
}
