package hyperframes

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
)

// ThumbWidth is the width of a thumbnail in pixels.
const ThumbWidth = 640

// Thumb is the kept thumbnail of a block (JPEG), or false when it has not been drawn yet.
func (s Store) Thumb(name string) ([]byte, bool) {
	dir, err := s.Path(name)
	if err != nil {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(dir, ThumbFile))
	return b, err == nil
}

// Thumbnail gives the thumbnail of a block, drawing and keeping it the first time: a frame well into the block (40% of its length), not
// at time zero where most of them have not started.
func (d Drawer) Thumbnail(ctx context.Context, name string) ([]byte, error) {
	if b, ok := d.Store.Thumb(name); ok {
		return b, nil
	}
	sess, blk, err := d.Open(ctx, name, "")
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	dur := blk.Duration
	if dur <= 0 {
		dur = 1
	}
	raw, err := sess.Frame(ctx, dur*0.4)
	if err != nil {
		return nil, err
	}
	jpg, err := shrink(raw, ThumbWidth)
	if err != nil {
		return nil, err
	}
	dir, _ := d.Store.Path(name)
	if err := os.WriteFile(filepath.Join(dir, ThumbFile), jpg, 0o644); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return jpg, nil
}

// shrink scales a PNG down to the width (never up) by averaging the pixels each new one covers, over white, and gives a JPEG.
func shrink(pngBytes []byte, width int) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > width {
		h = h * width / w
		w = width
	}
	if w < 1 || h < 1 {
		return nil, errors.New("empty picture")
	}
	flat := image.NewRGBA(b)
	draw.Draw(flat, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, b, src, b.Min, draw.Over)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, (y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, (x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := flat.RGBAAt(b.Min.X+xx, b.Min.Y+yy)
					r += uint32(c.R)
					g += uint32(c.G)
					bl += uint32(c.B)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
