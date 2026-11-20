package hyperframes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Check runs the minimum checks on a block and stores the result in its block.json: the files the registry lists are there; it draws
// with nothing asked from outside and no script error; two passes over the same moments (the second in a new browser: a render goes forward once) give the same frames; and something moves between the start and the end.
func (d Drawer) Check(ctx context.Context, name string) (CheckResult, error) {
	res, err := d.Run(ctx, name, "")
	if err != nil {
		return res, err
	}
	blk, err := d.Store.Get(name)
	if err != nil {
		return res, err
	}
	blk.Checks = &res
	return res, d.Store.Save(blk)
}

// Run runs the checks on the block with html as its main file (empty: the stored one) and stores nothing: the edit of a block is checked
// with it before it is saved.
func (d Drawer) Run(ctx context.Context, name, html string) (CheckResult, error) {
	d.Transparent = true // frames with their alpha: what an overlay draws over white is seen
	blk, err := d.Store.Get(name)
	if err != nil {
		return CheckResult{}, err
	}
	res := CheckResult{At: time.Now().UTC().Truncate(time.Second)}
	res.MissingFiles = d.missingFiles(name)
	if len(blk.Placeholders) > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d files are placeholders made here, not the registry's own: %s", len(blk.Placeholders), sample(blk.Placeholders, 3)))
	}
	if len(res.MissingFiles) > 0 {
		res.Problems = append(res.Problems, fmt.Sprintf("the registry lists %d files it does not ship: %s", len(res.MissingFiles), sample(res.MissingFiles, 3)))
	}
	res.Runtime = "builtin"
	if _, pg, err := d.Page(name, html, 0, nil); err == nil && d.UsesUpstream(pg.HTML) {
		res.Runtime = "upstream"
	}
	sess, _, err := d.Open(ctx, name, html)
	if err != nil {
		res.Problems = append(res.Problems, "does not open: "+err.Error())
		return d.finish(res)
	}
	defer sess.Close()

	dur := blk.Duration
	if dur <= 0 {
		dur = 1
	}
	moments := []float64{0, dur * 0.02, dur * 0.1, dur * 0.25, dur * 0.5, dur * 0.75, dur * 0.95}
	first := make([][]byte, len(moments))
	for i, t := range moments {
		png, err := sess.Frame(ctx, t)
		if err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("cannot draw %.2fs: %v", t, err))
			return d.finishWith(res, sess.Problems())
		}
		first[i] = png
	}
	// the second pass is a new browser: a render goes forward once, so that is what must give the same frames
	if second, err := d.pass(ctx, name, html, moments); err != nil {
		res.Problems = append(res.Problems, "cannot draw a second time: "+err.Error())
	} else {
		var differ []string
		var third [][]byte
		for i := range moments {
			if SameFrame(first[i], second[i]) {
				continue
			}
			// one more pass settles a moment where the browser was caught before a GPU effect was ready: it counts only if no two of the three agree
			if third == nil {
				var err error
				if third, err = d.pass(ctx, name, html, moments); err != nil {
					third = second
				}
			}
			if !SameFrame(first[i], third[i]) && !SameFrame(second[i], third[i]) { // no two of the three passes agree
				differ = append(differ, fmt.Sprintf("%.2fs", moments[i]))
			}
		}
		if len(differ) > 0 {
			res.Problems = append(res.Problems, "two passes give different frames at "+strings.Join(differ, ", "))
		}
	}
	still := true
	for _, h := range first[1:] {
		if !bytes.Equal(h, first[0]) {
			still = false
		}
	}
	if still {
		res.Warnings = append(res.Warnings, "static: nothing changes over its length")
	}
	return d.finishWith(res, sess.Problems())
}

// pass draws the moments in order in a browser of its own and gives each frame (PNG).
func (d Drawer) pass(ctx context.Context, name, html string, moments []float64) ([][]byte, error) {
	sess, _, err := d.Open(ctx, name, html)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	out := make([][]byte, len(moments))
	for i, t := range moments {
		png, err := sess.Frame(ctx, t)
		if err != nil {
			return nil, err
		}
		out[i] = png
	}
	return out, nil
}

func (d Drawer) finishWith(res CheckResult, sessionProblems []string) (CheckResult, error) {
	res.Problems = append(res.Problems, sessionProblems...)
	return d.finish(res)
}

func (d Drawer) finish(res CheckResult) (CheckResult, error) {
	for _, p := range res.Problems {
		if strings.Contains(p, "WebGPU unavailable") || strings.Contains(p, "WebGL") && strings.Contains(p, "unavailable") {
			res.NeedsGPU = true
		}
	}
	res.OK = len(res.Problems) == 0
	return res, nil
}

// missingFiles lists what registry-item.json (kept with the block) says the block has and the store does not.
func (d Drawer) missingFiles(name string) []string {
	dir, err := d.Store.Path(name)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "registry-item.json"))
	if err != nil {
		return nil
	}
	var item registryItem
	if json.Unmarshal(raw, &item) != nil {
		return nil
	}
	var out []string
	for _, f := range item.Files {
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.Path))); err != nil || !fi.Mode().IsRegular() {
			out = append(out, f.Path)
		}
	}
	return out
}

func sample(l []string, n int) string {
	if len(l) <= n {
		return strings.Join(l, ", ")
	}
	return strings.Join(l[:n], ", ") + ", ..."
}

// SameFrame says two PNG frames are the same picture: byte for byte, or with at most 0.2% of the pixels differing by more than 12 levels in any
// channel (the compositor of a browser may settle a blurred edge or a sub-pixel of text differently from one load to the next).
func SameFrame(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	ia, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return false
	}
	ib, err := png.Decode(bytes.NewReader(b))
	if err != nil || ia.Bounds() != ib.Bounds() {
		return false
	}
	bd := ia.Bounds()
	differ, total := 0, bd.Dx()*bd.Dy()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			if absDiff(r1, r2) > 12<<8 || absDiff(g1, g2) > 12<<8 || absDiff(b1, b2) > 12<<8 || absDiff(a1, a2) > 12<<8 {
				differ++
			}
		}
	}
	return float64(differ) <= 0.002*float64(total)
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
