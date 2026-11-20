package videoedit

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The engines `extend` can make its new clip with: those that take a first frame. These are the names of the API.
const (
	EngineMiniMaxH3 = "minimax-h3"
	EngineLTX       = "ltx"
)

// EngineOrder is the order the first available engine is looked for in: the one that follows a picture best first.
var EngineOrder = []string{EngineMiniMaxH3, EngineLTX}

// ClipEngine says whether an engine can make the new clip now, and why not when it cannot.
type ClipEngine struct {
	ID        string
	Available bool
	Reason    string
}

// ClipRequest asks for a clip that starts at a picture.
type ClipRequest struct {
	Engine string
	// Prompt says what happens; Picture is the first frame (a PNG, absolute path).
	Prompt, Picture string
	// Seconds is how long the clip should be at most; the engine's own clip may be shorter.
	Seconds float64
	// Width and Height are the video the clip is for, so the clip has its shape.
	Width, Height int
	GPU           string
	// Dir is a folder of the job's own, absolute, for what the engine writes.
	Dir  string
	Warn func(message string)
}

// ClipResult is the clip made: a video file (absolute path) and its length.
type ClipResult struct {
	File    string
	Seconds float64
}

// ClipMaker makes the clip of an extension with an engine of the machine. The server installs the real one, which
// plans it as a generation with a start picture and runs it; the tests install a stand-in.
type ClipMaker interface {
	Engines(ctx context.Context) []ClipEngine
	MakeClip(ctx context.Context, req ClipRequest) (ClipResult, error)
}

// PickEngine is the engine to use: the one asked for when it is available, else the first available in EngineOrder.
func PickEngine(engines []ClipEngine, asked string) (string, error) {
	by := map[string]ClipEngine{}
	for _, e := range engines {
		by[e.ID] = e
	}
	if asked != "" {
		e, ok := by[asked]
		switch {
		case !ok:
			return "", fmt.Errorf("the engine %s is not one that takes a first frame", asked)
		case !e.Available:
			return "", fmt.Errorf("the engine %s cannot run now: %s", asked, e.Reason)
		}
		return asked, nil
	}
	var reasons []string
	for _, id := range EngineOrder {
		if e, ok := by[id]; ok {
			if e.Available {
				return id, nil
			}
			reasons = append(reasons, id+": "+e.Reason)
		}
	}
	if len(reasons) == 0 {
		return "", fmt.Errorf("no engine that takes a first frame is set up")
	}
	return "", fmt.Errorf("no engine that takes a first frame can run now (%s)", strings.Join(reasons, "; "))
}

const extendedTag = "mo-extended="

// ExtendedTimes is how many times a video has been extended from its original, read from the comment the tool leaves
// in its result (0 for any other video).
func ExtendedTimes(info Info) int {
	for _, part := range strings.Fields(strings.ReplaceAll(info.Comment, ";", " ")) {
		if n, ok := strings.CutPrefix(part, extendedTag); ok {
			if v, err := strconv.Atoi(n); err == nil && v > 0 {
				return v
			}
		}
	}
	return 0
}

// lastFrameArgs extracts the last frame of a video as a PNG: ffmpeg is told to start a second before the end and to
// keep overwriting one picture, so the one left is the last.
func lastFrameArgs(video, out string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-y", "-sseof", "-1", "-i", video, "-an", "-update", "1", "-frames:v", "9999", out}
}

// joinArgs joins the source and the new clip into one video of the source's size and rate: the first frame of the
// clip (the picture the source ends on) is dropped so it is not shown twice, the clip is scaled to cover the source's
// frame and cut to it, and the count of extensions goes into the comment. The sound is not kept.
func joinArgs(source, clip, out string, info Info, times int) []string {
	rate := info.FPSText
	if rate == "" || rate == "0/0" {
		rate = trimFloat(info.FPS)
	}
	w, h := info.Width, info.Height
	graph := fmt.Sprintf("[0:v]setsar=1,fps=%s[a];[1:v]trim=start_frame=1,setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,crop=%d:%d,setsar=1,fps=%s[b];[a][b]concat=n=2:v=1:a=0[v]",
		rate, w, h, w, h, rate)
	return []string{"-hide_banner", "-loglevel", "error", "-y", "-i", source, "-i", clip, "-filter_complex", graph, "-map", "[v]", "-an",
		"-c:v", "libx264", "-preset", "medium", "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
		"-metadata", "comment=" + extendedTag + strconv.Itoa(times), out}
}

func (c *Catalog) runExtend(ctx context.Context, job Job) (Result, error) {
	maker := c.ClipMakerOf()
	if maker == nil {
		return Result{}, fmt.Errorf("no engine is set up to make the new clip")
	}
	prompt := Str(job.Settings, "prompt")
	if strings.TrimSpace(prompt) == "" {
		return Result{}, fmt.Errorf("the prompt is blank: say what happens next")
	}
	info, err := c.Probe(ctx, job.Video)
	if err != nil {
		return Result{}, err
	}
	engine, err := PickEngine(maker.Engines(ctx), Str(job.Settings, "engine"))
	if err != nil {
		return Result{}, err
	}
	dir, err := workDir(job, "extend")
	if err != nil {
		return Result{}, err
	}
	last := filepath.Join(dir, "last.png")
	if err := runFFmpeg(ctx, lastFrameArgs(job.Video, last)); err != nil {
		return Result{}, err
	}
	seconds := Float(job.Settings, "seconds")
	clip, err := maker.MakeClip(ctx, ClipRequest{
		Engine: engine, Prompt: prompt, Picture: last, Seconds: seconds, Width: info.Width, Height: info.Height,
		GPU: job.GPU, Dir: filepath.Join(dir, "clip"), Warn: job.Warn,
	})
	if err != nil {
		return Result{}, err
	}
	times := ExtendedTimes(info) + 1
	res := Result{Engine: engine, ExtendedTimes: times}
	if clip.Seconds > 0 && clip.Seconds < seconds-0.25 {
		msg := fmt.Sprintf("the engine's clip lasts %.1f s: the video is %.1f s longer, not the %g asked for", clip.Seconds, clip.Seconds, seconds)
		job.Warn(msg)
		res.Notes = append(res.Notes, msg)
	}
	if times > 1 {
		res.Notes = append(res.Notes, fmt.Sprintf("this video has been extended %d times: the quality drops with each extension", times))
	}
	if err := runFFmpeg(ctx, joinArgs(job.Video, clip.File, filepath.Join(job.OutDir, ResultName), info, times)); err != nil {
		return Result{}, err
	}
	return c.finish(ctx, job, res, ResultName, "mp4")
}

// extendAvailable is the availability of the tool: ffmpeg and an engine that takes a first frame.
func (c *Catalog) extendAvailable() (bool, string) {
	if ok, why := ffmpegAvailable(); !ok {
		return false, why
	}
	m := c.ClipMakerOf()
	if m == nil {
		return false, "no engine that takes a first frame is set up"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := PickEngine(m.Engines(ctx), ""); err != nil {
		return false, err.Error()
	}
	return true, ""
}
