package hyperframes

// placeholders.go: the files the registry lists for a block and does not ship (cover pictures, avatar clips, a soundtrack, textures, fonts). The
// store makes them so the block can be drawn: a picture the server generated (scripts/make-block-placeholders.sh keeps them in
// <store>/placeholders/), else a procedural one; a clip or a soundtrack with ffmpeg; a font from the pinned copy of the open font it is. Every
// file made is listed in block.json (placeholders), so nothing passes for the registry's own.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// PlaceholdersDir is where generated pictures are looked for, under the store.
const PlaceholdersDir = "placeholders"

var reFont = regexp.MustCompile(`^([a-z0-9-]+)-latin-([0-9]+)-normal\.woff2$`)

// FillMissing makes the files of block name that its registry item lists and the store does not have, and records them. It gives the paths made.
// Files it cannot make (a clip with no ffmpeg) stay missing, and the block stays `needs-files`.
func (s Store) FillMissing(name string) ([]string, error) {
	dir, err := s.Path(name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "registry-item.json"))
	if err != nil {
		return nil, err
	}
	var item registryItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	var made []string
	for _, f := range item.Files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		if fi, err := os.Stat(target); err == nil && fi.Mode().IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return made, err
		}
		if s.makeFile(f.Path, target) == nil {
			made = append(made, f.Path)
		}
	}
	if len(made) == 0 {
		return nil, nil
	}
	b, err := s.Get(name)
	if err != nil {
		return made, err
	}
	b.Placeholders = unionStrings(b.Placeholders, made)
	b.Files = unionStrings(b.Files, made)
	return made, s.Save(b)
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range [][]string{a, b} {
		for _, x := range l {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	return out
}

func (s Store) makeFile(rel, target string) error {
	base := path.Base(rel)
	switch strings.ToLower(path.Ext(rel)) {
	case ".jpg", ".jpeg", ".png":
		if b, err := os.ReadFile(filepath.Join(s.Dir, PlaceholdersDir, base)); err == nil {
			return os.WriteFile(target, b, 0o644)
		}
		return writePicture(rel, target)
	case ".mp4":
		return makeClip(s, target)
	case ".m4a", ".mp3", ".aac":
		return exec.Command("ffmpeg", "-y", "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "30", "-c:a", "aac", target).Run()
	case ".woff2":
		m := reFont.FindStringSubmatch(base)
		if m == nil {
			return fmt.Errorf("no copy of the font %s", base)
		}
		src, ok := Lookup(LibsDir, fmt.Sprintf("https://cdn.jsdelivr.net/fontsource/fonts/%s@5.2.5/latin-%s-normal.woff2", m[1], m[2]))
		if !ok {
			return fmt.Errorf("no copy of the font %s", base)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	}
	return fmt.Errorf("no way to make %s", rel)
}

// makeClip makes 4 s of a slow zoom over a generated picture (or a moving gradient when there is none), with a silent soundtrack.
func makeClip(s Store, target string) error {
	pics, _ := filepath.Glob(filepath.Join(s.Dir, PlaceholdersDir, "*.jpg"))
	args := []string{"-y", "-v", "error"}
	if len(pics) > 0 {
		args = append(args, "-loop", "1", "-i", pics[0], "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
			"-vf", "scale=1440:1440,zoompan=z='1+0.0015*on':d=100:s=720x720:fps=25,format=yuv420p", "-t", "4")
	} else {
		args = append(args, "-f", "lavfi", "-i", "gradients=s=720x720:d=4:speed=0.03:rate=25", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-vf", "format=yuv420p", "-t", "4")
	}
	args = append(args, "-c:v", "libx264", "-c:a", "aac", "-shortest", target)
	return exec.Command("ffmpeg", args...).Run()
}

// writePicture draws a procedural picture: a matcap (a lit sphere) for a name with matcap in it, a soft noise texture for a png, a gradient with a few
// soft shapes for a jpg, its colours taken from the name so each picture of a block looks different.
func writePicture(rel, target string) error {
	h := fnv.New32a()
	h.Write([]byte(rel))
	seed := h.Sum32()
	hue := float64(seed%360) / 360
	img := image.NewRGBA(image.Rect(0, 0, 768, 768))
	isMatcap := strings.Contains(strings.ToLower(rel), "matcap")
	for y := 0; y < 768; y++ {
		for x := 0; x < 768; x++ {
			u, v := float64(x)/767, float64(y)/767
			var c color.RGBA
			switch {
			case isMatcap:
				dx, dy := u-0.5, v-0.5
				r := math.Sqrt(dx*dx+dy*dy) * 2
				l := 0.25 + 0.7*math.Max(0, 1-r)*math.Max(0, 1-math.Hypot(u-0.38, v-0.34)*1.2)
				if r > 1 {
					l = 0.12
				}
				c = grey(l)
			case strings.HasSuffix(strings.ToLower(rel), ".png"):
				n := noise(u*6, v*6, seed)*0.6 + noise(u*14, v*14, seed+1)*0.4
				c = hsv(0.55+0.05*n, 0.25, 0.55+0.4*n)
			default:
				g := 0.5*u + 0.5*v
				n := noise(u*3, v*3, seed)
				c = hsv(math.Mod(hue+0.25*g+0.08*n, 1), 0.55+0.3*n, 0.35+0.55*(1-math.Abs(g-0.5)*1.2))
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if strings.HasSuffix(strings.ToLower(rel), ".png") {
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
	} else if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	return os.WriteFile(target, buf.Bytes(), 0o644)
}

func grey(l float64) color.RGBA {
	v := uint8(math.Max(0, math.Min(1, l)) * 255)
	return color.RGBA{v, v, v, 255}
}

func hsv(h, s, v float64) color.RGBA {
	h = math.Mod(h, 1) * 6
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}

// noise is smooth value noise in 0..1.
func noise(x, y float64, seed uint32) float64 {
	ix, iy := math.Floor(x), math.Floor(y)
	fx, fy := x-ix, y-iy
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	r := func(a, b float64) float64 {
		h := fnv.New32a()
		fmt.Fprintf(h, "%d/%g/%g", seed, a, b)
		return float64(h.Sum32()%10000) / 10000
	}
	a, b := r(ix, iy), r(ix+1, iy)
	c, d := r(ix, iy+1), r(ix+1, iy+1)
	return a*(1-fx)*(1-fy) + b*fx*(1-fy) + c*(1-fx)*fy + d*fx*fy
}
