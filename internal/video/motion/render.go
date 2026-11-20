package motion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync"
)

// Job is a page to render to a video file.
type Job struct {
	Page    Page
	FPS     int
	Browser string
	// FFmpeg is the ffmpeg program; empty means "ffmpeg".
	FFmpeg string
	// Out is the file to write. A transparent page needs a .webm (VP9 with alpha) or .mov (ProRes 4444).
	Out    string
	Limits Limits
}

// Frames is the number of frames of a video of duration seconds at fps.
func Frames(duration float64, fps int) int {
	n := int(math.Round(duration * float64(fps)))
	if n < 1 {
		n = 1
	}
	return n
}

// Render plays the page frame by frame, setting its time before each capture, and
// encodes the frames to Job.Out. It returns the number of frames and the problems the
// page reported (script errors, blocked requests); the video is written even then.
func Render(ctx context.Context, j Job) (frames int, problems []string, err error) {
	if j.FPS <= 0 {
		return 0, nil, errors.New("the frame rate must be positive")
	}
	if j.Page.Duration <= 0 {
		return 0, nil, errors.New("the duration must be positive")
	}
	args, err := encodeArgs(j)
	if err != nil {
		return 0, nil, err
	}
	s, err := Open(ctx, j.Browser, j.Page, j.Limits)
	if err != nil {
		return 0, nil, err
	}
	defer s.Close()

	ff := j.FFmpeg
	if ff == "" {
		ff = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return 0, nil, err
	}
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("cannot start ffmpeg: %w", err)
	}
	n := Frames(j.Page.Duration, j.FPS)
	var ferr error
	for i := 0; i < n; i++ {
		png, e := s.Frame(ctx, float64(i)/float64(j.FPS))
		if e != nil {
			ferr = e
			break
		}
		if _, e := in.Write(png); e != nil {
			ferr = fmt.Errorf("ffmpeg stopped reading frames: %w", e)
			break
		}
	}
	in.Close()
	werr := cmd.Wait()
	if ferr != nil {
		return 0, s.Problems(), ferr
	}
	if werr != nil {
		return 0, s.Problems(), fmt.Errorf("ffmpeg failed: %w: %s", werr, lastLines(stderr.String(), 6))
	}
	return n, s.Problems(), nil
}

// encodeArgs is the ffmpeg command line that reads PNG frames from stdin.
func encodeArgs(j Job) ([]string, error) {
	in := []string{"-y", "-hide_banner", "-loglevel", "error", "-f", "image2pipe", "-framerate", fmt.Sprint(j.FPS), "-c:v", "png", "-i", "-"}
	switch {
	case j.Page.Transparent && strings.HasSuffix(j.Out, ".webm"):
		return append(in, "-c:v", "libvpx-vp9", "-pix_fmt", "yuva420p", "-b:v", "0", "-crf", "28", "-auto-alt-ref", "0", j.Out), nil
	case j.Page.Transparent && strings.HasSuffix(j.Out, ".mov"):
		return append(in, "-c:v", "prores_ks", "-profile:v", "4444", "-pix_fmt", "yuva444p10le", j.Out), nil
	case j.Page.Transparent:
		return nil, fmt.Errorf("a transparent video needs a .webm or .mov file, not %q", j.Out)
	default:
		return append(in, "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-pix_fmt", "yuv420p",
			"-crf", "18", "-preset", "medium", "-movflags", "+faststart", j.Out), nil
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

var (
	overlayExtOnce sync.Once
	overlayExt     string
	overlayExtErr  error
)

// OverlayExt is the file extension a transparent video is written with on this machine:
// "webm" (VP9 with alpha) when ffmpeg has the encoder, else "mov" (ProRes 4444), else an error.
func OverlayExt() (string, error) {
	overlayExtOnce.Do(func() {
		out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
		if err != nil {
			overlayExtErr = fmt.Errorf("cannot ask ffmpeg for its encoders: %w", err)
			return
		}
		switch {
		case bytes.Contains(out, []byte("libvpx-vp9")):
			overlayExt = "webm"
		case bytes.Contains(out, []byte("prores_ks")):
			overlayExt = "mov"
		default:
			overlayExtErr = errors.New("this ffmpeg can encode neither VP9 (libvpx-vp9) nor ProRes (prores_ks), so overlays with transparency cannot be made")
		}
	})
	return overlayExt, overlayExtErr
}
