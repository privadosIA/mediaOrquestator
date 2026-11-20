package quality

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
)

// Box is a rectangle in pixels of the frame it was found in.
type Box struct{ X0, Y0, X1, Y1 int }

const (
	maxFaces    = 4   // faces judged per frame
	faceMinSide = 336 // crops are enlarged to at least this many pixels
)

// judgeFaces crops every face of the frame, enlarges it and judges it. It
// returns the worst verdict, or nil when there are no faces.
func judgeFaces(ctx context.Context, f FaceFinder, img image.Image, data []byte) (*Judgement, error) {
	boxes, err := f.Faces(ctx, data)
	if err != nil {
		return nil, err
	}
	if len(boxes) > maxFaces {
		boxes = boxes[:maxFaces]
	}
	var worst *Judgement
	for _, b := range boxes {
		crop := cropFace(img, b)
		if crop == nil {
			continue
		}
		var buf bytes.Buffer
		if png.Encode(&buf, crop) != nil {
			continue
		}
		j, err := f.JudgeFace(ctx, buf.Bytes())
		if err != nil {
			return worst, err
		}
		if j.Defect && j.Issue != "" {
			j.Issue = "face: " + j.Issue
		}
		if worst == nil || (j.Defect && (!worst.Defect || j.Severity > worst.Severity)) {
			jj := j
			worst = &jj
		}
	}
	return worst, nil
}

// cropFace cuts the box with a margin around it and enlarges it (nearest
// neighbour) so its shorter side is at least faceMinSide. It returns nil for an
// empty box.
func cropFace(img image.Image, b Box) image.Image {
	bounds := img.Bounds()
	w, h := b.X1-b.X0, b.Y1-b.Y0
	if w < 4 || h < 4 {
		return nil
	}
	mx, my := w/2, h/4
	r := image.Rect(b.X0-mx, b.Y0-my, b.X1+mx, b.Y1+my).Intersect(bounds)
	if r.Empty() {
		return nil
	}
	scale := 1
	for min(r.Dx(), r.Dy())*scale < faceMinSide && scale < 12 {
		scale++
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx()*scale, r.Dy()*scale))
	src := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(src, src.Bounds(), img, r.Min, draw.Src)
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			out.Set(x, y, src.At(x/scale, y/scale))
		}
	}
	return out
}
