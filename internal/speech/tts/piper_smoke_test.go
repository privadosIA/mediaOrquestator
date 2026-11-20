package tts

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// realRunner runs the actual binary, so the smoke test exercises Piper's
// real flag handling rather than a fake.
type realRunner struct{ path string }

func (r realRunner) Run(ctx context.Context, _ string, args []string, stdin io.Reader, stdout io.Writer) (string, error) {
	cmd := exec.CommandContext(ctx, r.path, args...)
	cmd.Stdin, cmd.Stdout = stdin, stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// TestSynthesizeWithRealPiper is the check that would have caught the three
// original defects (missing --model, WAV header overwritten by Piper's
// stdout, doubled path). It needs the piper binary on PATH (or in the
// repo's bin/), a voice with its .onnx.json under the repo's voices/, and
// ffprobe; otherwise it skips.
func TestSynthesizeWithRealPiper(t *testing.T) {
	repo := filepath.Join("..", "..", "..")
	piper, err := exec.LookPath("piper")
	if err != nil {
		local := filepath.Join(repo, "bin", "piper")
		if _, statErr := os.Stat(local); statErr != nil {
			t.Skip("piper not available")
		}
		piper = local
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not available")
	}
	voices, _ := filepath.Glob(filepath.Join(repo, "voices", "*.onnx"))
	var voice string
	for _, v := range voices {
		if _, err := os.Stat(v + ".json"); err == nil {
			voice = v
			break
		}
	}
	if voice == "" {
		t.Skip("no Piper voice with its .onnx.json under voices/")
	}
	// Piper's shared libraries sit next to the binary.
	if dir := filepath.Dir(piper); dir != "." {
		abs, _ := filepath.Abs(dir)
		t.Setenv("LD_LIBRARY_PATH", abs+":"+os.Getenv("LD_LIBRARY_PATH"))
	}

	out := filepath.Join(t.TempDir(), "scene.wav")
	if err := Synthesize(context.Background(), realRunner{piper}, "Hello from the smoke test.", 1.0, voice, out); err != nil {
		t.Fatalf("Synthesize with real piper: %v", err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 44 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("not a WAV: first bytes %q", b[:min(len(b), 12)])
	}
	dur, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", out).Output()
	if err != nil {
		t.Fatalf("ffprobe rejected the WAV: %v", err)
	}
	if secs, err := strconv.ParseFloat(strings.TrimSpace(string(dur)), 64); err != nil || secs <= 0 {
		t.Fatalf("duration %q, want > 0", dur)
	}
}
