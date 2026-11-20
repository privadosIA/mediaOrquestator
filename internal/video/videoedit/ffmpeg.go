package videoedit

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResultName is the file every ffmpeg tool leaves in the output folder.
const ResultName = "result.mp4"

// even rounds a size to the nearest even number (never below 2): H.264 in yuv420p needs even sides.
func even(v float64) int {
	n := int(math.Round(v/2)) * 2
	if n < 2 {
		n = 2
	}
	return n
}

// evenUp is even rounding up, for a canvas that must hold a whole picture.
func evenUp(v float64) int {
	n := int(math.Ceil(v/2-1e-9)) * 2
	if n < 2 {
		n = 2
	}
	return n
}

// encodeArgs is the tail every tool shares: H.264 in yuv420p with the moov atom first, and either the audio
// re-encoded as AAC (keepAudio, when the source has any) or none.
func encodeArgs(keepAudio bool) []string {
	args := []string{"-map", "0:v:0"}
	if keepAudio {
		args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-an")
	}
	return append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
}

// ffmpegArgs builds the whole command: input, the video filter, the encoding, the output.
func ffmpegArgs(in, filter, out string, keepAudio bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", in}
	if filter != "" {
		args = append(args, "-vf", filter)
	}
	args = append(args, encodeArgs(keepAudio)...)
	return append(args, out)
}

// runFFmpeg runs ffmpeg and, when it fails, reports the end of what it said.
func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return fmt.Errorf("ffmpeg failed: %w: %s", err, msg)
	}
	return nil
}

// finish describes the file a tool made by reading it back. The file is named name in the output folder.
func (c *Catalog) finish(ctx context.Context, job Job, res Result, name, format string) (Result, error) {
	res.File, res.Format = name, format
	info, err := c.Probe(ctx, filepath.Join(job.OutDir, name))
	if err != nil {
		return Result{}, fmt.Errorf("cannot read the result: %w", err)
	}
	res.Width, res.Height, res.FPS, res.Duration, res.HasAudio = info.Width, info.Height, info.FPS, info.Duration, info.HasAudio
	return res, nil
}

// --- scale ----------------------------------------------------------------------------------

// scaleFilter makes the picture factor times as large with lanczos, to even sides.
func scaleFilter(info Info, factor float64) (filter string, w, h int) {
	w, h = even(float64(info.Width)*factor), even(float64(info.Height)*factor)
	return fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h), w, h
}

func (c *Catalog) runScale(ctx context.Context, job Job) (Result, error) {
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	res := Result{Engine: "fast"}
	if Str(job.Settings, "engine") == "quality" {
		// The network when it is installed and works; otherwise the edit still finishes, with the resampling filter, and
		// says why.
		var why string
		if ok, reason := c.RealESRGANInstalled(); !ok {
			why = reason + " (" + upscalerHint + ")"
		} else if r, err := c.scaleWithNetwork(ctx, job, info, Int(job.Settings, "factor")); err == nil {
			return c.finish(ctx, job, r, ResultName, "mp4")
		} else if ctx.Err() != nil {
			return Result{}, ctx.Err()
		} else {
			why = err.Error()
		}
		job.Warn("the quality engine could not be used (" + why + "): scaled with the lanczos filter")
		res.Notes = append(res.Notes, "asked for the quality engine; made with the fast one (lanczos) because "+why)
	}
	filter, _, _ := scaleFilter(info, Float(job.Settings, "factor"))
	if err := runFFmpeg(ctx, ffmpegArgs(job.Video, filter, filepath.Join(job.OutDir, ResultName), true)); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, res, ResultName, "mp4")
}

// --- interpolate -------------------------------------------------------------------------------

// interpolateFilter makes the frames in between with motion-compensated interpolation.
func interpolateFilter(fps int) string {
	return fmt.Sprintf("minterpolate=fps=%d:mi_mode=mci:mc_mode=aobmc:me_mode=bidir:vsbmc=1", fps)
}

func (c *Catalog) runInterpolate(ctx context.Context, job Job) (Result, error) {
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	target := Int(job.Settings, "fps")
	if float64(target) <= math.Round(info.FPS) {
		return Result{}, fmt.Errorf("the video has %.4g frames per second: a target of %d is not above it", info.FPS, target)
	}
	res := Result{Engine: "minterpolate"}
	if ok, reason := c.RIFEInstalled(); !ok {
		res.Notes = append(res.Notes, "made with a filter (minterpolate) because "+reason+" ("+upscalerHint+")")
		job.Warn("the interpolation network is not available (" + reason + "): the frames in between are made with a filter")
	} else if r, err := c.interpolateWithNetwork(ctx, job, info, target); err == nil {
		return c.finish(ctx, job, r, ResultName, "mp4")
	} else if ctx.Err() != nil {
		return Result{}, ctx.Err()
	} else {
		res.Notes = append(res.Notes, "made with a filter (minterpolate) because the network could not be used: "+err.Error())
		job.Warn("the interpolation network could not be used (" + err.Error() + "): the frames in between are made with a filter")
	}
	// The sound is dropped: the tool is meant for a picture that will be used on its own, and the spec lists the
	// tools that keep it (scale, reframe, remove-background).
	if err := runFFmpeg(ctx, ffmpegArgs(job.Video, interpolateFilter(target), filepath.Join(job.OutDir, ResultName), false)); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, res, ResultName, "mp4")
}

// --- reframe -----------------------------------------------------------------------------------

// aspectOf reads "16:9" as a ratio.
func aspectOf(s string) (float64, error) {
	var a, b float64
	if _, err := fmt.Sscanf(s, "%f:%f", &a, &b); err != nil || a <= 0 || b <= 0 {
		return 0, fmt.Errorf("aspect ratio %q is not a:b", s)
	}
	return a / b, nil
}

// cropWindow is the largest window of the ratio inside the picture: it keeps the height of the source, or the
// width when that makes it smaller.
func cropWindow(w, h int, ratio float64) (cw, ch int) {
	if float64(w)/float64(h) > ratio { // wider than asked: keep the height
		ch = h &^ 1
		cw = even(float64(ch) * ratio)
		if cw > w {
			cw = w &^ 1
		}
		return cw, ch
	}
	cw = w &^ 1
	ch = even(float64(cw) / ratio)
	if ch > h {
		ch = h &^ 1
	}
	return cw, ch
}

// padCanvas is the smallest canvas of the ratio that holds the whole picture.
func padCanvas(w, h int, ratio float64) (cw, ch int) {
	if float64(w)/float64(h) > ratio { // wider than asked: bars above and below
		return evenUp(float64(w)), evenUp(float64(w) / ratio)
	}
	return evenUp(float64(h) * ratio), evenUp(float64(h))
}

// reframeFilter builds the filter that gives the picture the ratio, cutting or adding bars. The window of a crop
// is centred; the pad bars take the colour.
func reframeFilter(info Info, ratio float64, mode, color string) (filter string, w, h int) {
	if mode == "pad" {
		cw, ch := padCanvas(info.Width, info.Height, ratio)
		c := "0x" + strings.TrimPrefix(color, "#")
		return fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=%s", cw, ch, c), cw, ch
	}
	cw, ch := cropWindow(info.Width, info.Height, ratio)
	return fmt.Sprintf("crop=%d:%d:(iw-%d)/2:(ih-%d)/2", cw, ch, cw, ch), cw, ch
}

func (c *Catalog) runReframe(ctx context.Context, job Job) (Result, error) {
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	ratio, err := aspectOf(Str(job.Settings, "aspect_ratio"))
	if err != nil {
		return Result{}, err
	}
	mode := Str(job.Settings, "mode")
	filter, _, _ := reframeFilter(info, ratio, mode, Str(job.Settings, "color"))
	res := Result{Engine: mode}
	if mode == "crop" {
		// The window follows the person when the vision service can say where they are; otherwise it stays centred
		// and the result says why.
		follow, note, err := c.followFilter(ctx, job, info, ratio)
		switch {
		case err != nil:
			return Result{}, err
		case follow != "":
			filter, res.Engine = follow, "crop-follow"
		case note != "":
			job.Warn(note)
			res.Notes = append(res.Notes, note)
		}
	}
	if err := runFFmpeg(ctx, ffmpegArgs(job.Video, filter, filepath.Join(job.OutDir, ResultName), true)); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, res, ResultName, "mp4")
}

// ffmpegAvailable says whether ffmpeg and ffprobe are on the PATH.
func ffmpegAvailable() (bool, string) {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return false, bin + " is not installed"
		}
	}
	return true, ""
}
