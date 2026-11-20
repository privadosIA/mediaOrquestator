package videoedit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// frameSide is the longest side of the frames the networks are shown: both read a smaller picture than that anyway,
// and the results (a mask, a depth) are brought back to the video's size by ffmpeg.
const frameSide = 640

// workDir is a folder of this job's own inside the project's tmp, which the pipeline clears when the run ends.
func workDir(job Job, name string) (string, error) {
	dir := filepath.Join(job.Dir, "tmp", name)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return dir, os.MkdirAll(dir, 0o755)
}

// extractFrames writes the frames of a video as PNG (f_00001.png ...) into dir, at fps frames per second (the
// video's own when it is 0) and with the long side at most frameSide. It returns how many it wrote.
func extractFrames(ctx context.Context, video, dir string, fps float64) (int, error) {
	filters := []string{}
	if fps > 0 {
		filters = append(filters, fmt.Sprintf("fps=%s", trimFloat(fps)))
	}
	filters = append(filters, fmt.Sprintf("scale='if(gt(iw,ih),min(%d,iw),-2)':'if(gt(iw,ih),-2,min(%d,ih))'", frameSide, frameSide))
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", video, "-an", "-vf", strings.Join(filters, ","), filepath.Join(dir, "f_%05d.png")}
	if err := runFFmpeg(ctx, args); err != nil {
		return 0, err
	}
	files, _ := filepath.Glob(filepath.Join(dir, "f_*.png"))
	if len(files) == 0 {
		return 0, fmt.Errorf("no frame could be read from the video")
	}
	return len(files), nil
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.4f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func (c *Catalog) requireVision() (Vision, error) {
	v := c.VisionOf()
	if v == nil {
		return nil, fmt.Errorf("the vision service is not set up")
	}
	return v, nil
}

// --- remove-background -----------------------------------------------------------------------------

// ffmpegColor writes #rrggbb as ffmpeg reads it.
func ffmpegColor(c string) string { return "0x" + strings.TrimPrefix(c, "#") }

// removeBackgroundArgs builds the command that puts the people of the video (the grey masks of the second input,
// brought to the video's size) over a plain colour, or over transparency in a WebM.
func removeBackgroundArgs(video, maskPattern, out string, info Info, alpha bool, color string) []string {
	fps := trimFloat(info.FPS)
	mask := fmt.Sprintf("[1:v]scale=%d:%d:flags=bicubic,format=gray[m]", info.Width, info.Height)
	var graph string
	if alpha {
		graph = mask + ";[0:v]format=yuva420p[v];[v][m]alphamerge[out]"
	} else {
		graph = mask + fmt.Sprintf(";[0:v]format=rgb24[v];[v][m]alphamerge[fg];color=c=%s:s=%dx%d:r=%s[bg];[bg][fg]overlay=shortest=1:format=auto,format=yuv420p[out]",
			ffmpegColor(color), info.Width, info.Height, fps)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", video, "-framerate", fps, "-i", maskPattern,
		"-filter_complex", graph, "-map", "[out]", "-map", "0:a:0?"}
	if alpha {
		args = append(args, "-c:v", "libvpx-vp9", "-pix_fmt", "yuva420p", "-b:v", "0", "-crf", "28", "-auto-alt-ref", "0", "-c:a", "libopus", "-b:a", "128k")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-c:a", "aac", "-b:a", "192k")
	}
	return append(args, "-shortest", out)
}

func (c *Catalog) runRemoveBackground(ctx context.Context, job Job) (Result, error) {
	v, err := c.requireVision()
	if err != nil {
		return Result{}, err
	}
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	alpha := Bool(job.Settings, "alpha")
	name, format := ResultName, "mp4"
	if alpha {
		name, format = "result.webm", "webm"
	}
	frames, err := workDir(job, "frames")
	if err != nil {
		return Result{}, err
	}
	masks, err := workDir(job, "masks")
	if err != nil {
		return Result{}, err
	}
	fps := info.FPS
	if fps <= 0 {
		fps = 25
	}
	info.FPS = fps
	n, err := extractFrames(ctx, job.Video, frames, fps)
	if err != nil {
		return Result{}, err
	}
	sum, err := v.Mask(ctx, frames, masks)
	if err != nil {
		return Result{}, err
	}
	res := Result{Engine: "u2net"}
	out := filepath.Join(job.OutDir, name)
	without := 0
	if sum.FramesWithoutPeople != nil {
		without = *sum.FramesWithoutPeople
	}
	switch {
	case without >= n:
		// Nobody anywhere: the video comes out as it was, and says so. It is not a failure.
		msg := "no person was found in the video: the result is the video unchanged"
		job.Warn(msg)
		res.Notes = append(res.Notes, msg)
		if err := runFFmpeg(ctx, ffmpegArgs(job.Video, "", filepath.Join(job.OutDir, ResultName), true)); err != nil {
			return Result{}, err
		}
		return c.finish(ctx, job, res, ResultName, "mp4")
	case without > 0:
		res.Notes = append(res.Notes, fmt.Sprintf("no person was found in %d of the %d frames: those show only the background", without, n))
	}
	if err := runFFmpeg(ctx, removeBackgroundArgs(job.Video, filepath.Join(masks, "f_%05d.png"), out, info, alpha, Str(job.Settings, "background"))); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, res, name, format)
}

// --- depth ----------------------------------------------------------------------------------------

// depthArgs assembles the grey frames of the depth into a video of the source's size and rate. The sound is not
// kept: the picture is no longer what the sound was made for.
func depthArgs(pattern, out string, info Info) []string {
	filter := fmt.Sprintf("scale=%d:%d:flags=bicubic,format=gray,format=yuv420p", info.Width, info.Height)
	return []string{"-hide_banner", "-loglevel", "error", "-y", "-framerate", trimFloat(info.FPS), "-i", pattern, "-vf", filter,
		"-an", "-c:v", "libx264", "-preset", "medium", "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart", out}
}

func (c *Catalog) runDepth(ctx context.Context, job Job) (Result, error) {
	v, err := c.requireVision()
	if err != nil {
		return Result{}, err
	}
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	if info.FPS <= 0 {
		info.FPS = 25
	}
	frames, err := workDir(job, "frames")
	if err != nil {
		return Result{}, err
	}
	depths, err := workDir(job, "depth")
	if err != nil {
		return Result{}, err
	}
	if _, err := extractFrames(ctx, job.Video, frames, info.FPS); err != nil {
		return Result{}, err
	}
	if _, err := v.Depth(ctx, frames, depths); err != nil {
		return Result{}, err
	}
	if err := runFFmpeg(ctx, depthArgs(filepath.Join(depths, "f_%05d.png"), filepath.Join(job.OutDir, ResultName), info)); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, Result{Engine: "depth-anything-v2-small"}, ResultName, "mp4")
}

// availableWithVisionAndFFmpeg is the availability of a tool that needs both ffmpeg and the vision service.
func (c *Catalog) availableWithVisionAndFFmpeg() (bool, string) {
	if ok, why := ffmpegAvailable(); !ok {
		return false, why
	}
	return c.NeedsVision()
}
