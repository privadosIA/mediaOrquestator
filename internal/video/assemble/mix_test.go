package assemble

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMixArgs_CopiesTheVideoAndLoopsTheMusic(t *testing.T) {
	args := BuildMixArgs("tmp/v.mp4", "sources/m.mp3", "out/f.mp4", MixOptions{Volume: 0.3, Fade: 2, Ducking: true, Duration: 30})
	line := strings.Join(args, " ")
	for _, want := range []string{"-i tmp/v.mp4", "-stream_loop -1 -i sources/m.mp3", "-map 0:v", "-map [a]", "-c:v copy", "-c:a aac", "-b:a 192k", "-ar 44100", "-ac 2"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if args[len(args)-1] != "out/f.mp4" {
		t.Fatal("output must be last")
	}
}

func TestMixFilter(t *testing.T) {
	f := MixFilter(MixOptions{Volume: 0.3, Fade: 2, Ducking: true, Duration: 30})
	for _, want := range []string{"volume=0.3", "afade=t=in:st=0:d=2", "afade=t=out:st=28:d=2", "sidechaincompress", "asplit=2", "amix=inputs=2:duration=first:normalize=0", "loudnorm=I=-16:TP=-1.5:LRA=11"} {
		if !strings.Contains(f, want) {
			t.Errorf("missing %q in %s", want, f)
		}
	}
	plain := MixFilter(MixOptions{Volume: 0.5, Fade: 0, Ducking: false, Duration: 30})
	if strings.Contains(plain, "sidechaincompress") || strings.Contains(plain, "afade") {
		t.Errorf("no ducking and no fade: %s", plain)
	}
}

func TestMixFilter_AShortVideoNeverFadesLongerThanHalfOfItself(t *testing.T) {
	f := MixFilter(MixOptions{Volume: 0.3, Fade: 2, Duration: 3})
	if !strings.Contains(f, "afade=t=in:st=0:d=1.5") || !strings.Contains(f, "afade=t=out:st=1.5:d=1.5") {
		t.Fatal(f)
	}
}

func TestMixMusic_ReportsTheFailure(t *testing.T) {
	fr := &fakeRunner{err: context.Canceled, stderr: "boom"}
	stderr, err := MixMusic(context.Background(), fr, "v", "m", "o", MixOptions{Duration: 5})
	if err == nil || stderr != "boom" {
		t.Fatalf("%v %q", err, stderr)
	}
}

type execRunner struct{ dir string }

func (e execRunner) Run(ctx context.Context, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Dir = e.dir
	out, err := cmd.CombinedOutput()
	return "", string(out), err
}

// TestMixMusic_RealFFmpeg proves the graph is accepted by ffmpeg and yields a
// video with its original duration and one stereo audio stream.
func TestMixMusic_RealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	r := execRunner{dir: dir}
	gen := func(args ...string) {
		t.Helper()
		if _, e, err := r.Run(context.Background(), args); err != nil {
			t.Fatalf("%v: %s", err, e)
		}
	}
	// 6 s video with a "narration" tone only in the first 2 s, 3 s of music (shorter: must loop).
	gen("-y", "-f", "lavfi", "-i", "color=c=black:s=64x64:d=6:r=10", "-f", "lavfi", "-i", "sine=f=440:d=2", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-filter_complex", "[2:a][1:a]amix=inputs=2:duration=longest[a]", "-map", "0:v", "-map", "[a]", "-t", "6", "-c:v", "libx264", "-c:a", "aac", "-ar", "44100", "-ac", "2", "v.mp4")
	gen("-y", "-f", "lavfi", "-i", "sine=f=220:d=3", "-ar", "48000", "m.wav")
	for _, ducking := range []bool{true, false} {
		out := filepath.Join(dir, "out.mp4")
		if stderr, err := MixMusic(context.Background(), r, "v.mp4", "m.wav", "out.mp4", MixOptions{Volume: 0.4, Fade: 1, Ducking: ducking, Duration: 6}); err != nil {
			t.Fatalf("ducking=%v: %v\n%s", ducking, err, stderr)
		}
		cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,channels,sample_rate:format=duration", "-of", "default=nw=1", out)
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v %s", err, b)
		}
		got := string(b)
		for _, want := range []string{"codec_name=h264", "codec_name=aac", "channels=2", "sample_rate=44100"} {
			if !strings.Contains(got, want) {
				t.Errorf("ducking=%v: missing %s in %s", ducking, want, got)
			}
		}
		if !strings.Contains(got, "duration=6.") && !strings.Contains(got, "duration=5.9") {
			t.Errorf("ducking=%v: the video length must be kept: %s", ducking, got)
		}
	}
}

// TestFitAndRotate_RealFFmpeg proves ffmpeg accepts the fit and the rotation
// graphs and that the clip keeps the configured resolution.
func TestFitAndRotate_RealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	r := execRunner{dir: dir}
	run := func(args ...string) {
		t.Helper()
		if _, e, err := r.Run(context.Background(), args); err != nil {
			t.Fatalf("%v: %s", err, e)
		}
	}
	run("-y", "-f", "lavfi", "-i", "testsrc=s=800x600:d=1", "-frames:v", "1", "photo.jpg")
	run(BuildFitImageArgs("photo.jpg", 320, 180, "fit.png")...)
	run("-y", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1", "a.wav")
	for _, mt := range []string{"rotate_cw", "rotate_ccw"} {
		scene := Scene{ID: "s", ImagePath: "fit.png", AudioPath: "a.wav", Duration: 1, MotionType: mt, MotionIntensity: 1, Width: 320, Height: 180}
		run(BuildClipArgs(scene, 10, mt+".mp4")...)
		out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", filepath.Join(dir, mt+".mp4")).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "320,180" {
			t.Fatalf("%s: %v %q", mt, err, out)
		}
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=width,height", "-of", "csv=p=0", filepath.Join(dir, "fit.png")).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "320,180" {
		t.Fatalf("fit: %v %q", err, out)
	}
}
