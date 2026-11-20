package quality

import (
	"context"
	"image"
	"strings"
	"testing"
)

func TestParseFaces_AcceptsTheShapesTheModelUses(t *testing.T) {
	want := []Box{{100, 50, 200, 150}}
	for _, in := range []string{
		`{"faces":[{"bbox_2d":[100,100,200,300],"label":"face"}]}`,
		`[{"bbox_2d":[100,100,200,300]}]`,
		`{"bbox_2d":[100,100,200,300],"label":"face"}`,
	} {
		got := parseFaces(in, 1000, 500)
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("%s -> %v", in, got)
		}
	}
	for _, in := range []string{`{"faces":[]}`, `not json`, `{"faces":[{"bbox_2d":[1,2,3]}]}`, `{"faces":[{"bbox_2d":[5,5,5,9]}]}`} {
		if got := parseFaces(in, 100, 100); len(got) != 0 {
			t.Errorf("%s -> %v", in, got)
		}
	}
}

func TestCropFace_EnlargesSmallFaces(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	c := cropFace(img, Box{100, 100, 130, 140})
	if c == nil || c.Bounds().Dy() < faceMinSide && c.Bounds().Dx() < faceMinSide {
		t.Fatalf("small face not enlarged: %v", c)
	}
	if cropFace(img, Box{10, 10, 12, 12}) != nil {
		t.Error("a 2 px box is not a face")
	}
}

type faceVision struct{ scripted }

func (faceVision) Faces(context.Context, []byte) ([]Box, error) {
	return []Box{{300, 200, 340, 260}}, nil
}
func (faceVision) JudgeFace(context.Context, []byte) (Judgement, error) {
	return Judgement{Defect: true, Severity: 9, Issue: "melted mouth"}, nil
}

func TestCheckClip_DeformedFaceInAWideShotIsCaught(t *testing.T) {
	path := clip(t, "a.mp4", steady)
	// the whole-frame judge sees nothing; the face crop does
	rep := check(t, path, &faceVision{scripted{verdict: func(int) Judgement { return Judgement{} }}})
	if rep.Verdict == VerdictOK || rep.Kind != "vision" || !strings.Contains(rep.Summary(), "face: melted mouth") {
		t.Errorf("verdict %s kind %s: %s", rep.Verdict, rep.Kind, rep.Summary())
	}
}
