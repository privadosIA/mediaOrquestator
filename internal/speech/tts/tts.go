// Package tts turns the text of a scene into a WAV file with Piper. The call goes through a [Runner], so a test never
// starts the program; [Flags] names every Piper flag in one place, in case a new version renames one.
package tts

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// Flags centralizes every Piper CLI flag in one place so they are easy to
// correct later (see design.md).
var Flags = struct {
	LengthScale string
	Model       string
	OutputFile  string
	Speaker     string
}{
	LengthScale: "--length-scale",
	Model:       "--model",
	OutputFile:  "--output_file",
	Speaker:     "--speaker",
}

// StdoutTarget asks Piper to emit the WAV on stdout instead of a file.
const StdoutTarget = "-"

// Binary is the Piper executable name.
const Binary = "piper"

// Runner abstracts external command execution.
// The concrete implementation lives in internal/platform/runner.
type Runner interface {
	Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout io.Writer) (stderr string, err error)
}

// Synthesize invokes the Piper TTS binary via the supplied Runner.
//
// It feeds ttsText over stdin, selects the voice with --model, asks Piper
// for the WAV on stdout (--output_file -) and writes that stream to
// outPath. No file path is ever handed to Piper: outPath is created
// Go-side only, so callers can pass a path resolved against their own
// working directory while Piper runs in another one. voice is passed
// exactly as given (Piper resolves it against its cwd). The speed field
// becomes Piper's length_scale = 1/speed. All CLI flags come from Flags.
//
// If Piper succeeds but emits no bytes, outPath is removed and an error is
// returned. Any error returned by the Runner (including captured stderr)
// is propagated unchanged, and the partial file is removed.
func Synthesize(ctx context.Context, r Runner, ttsText string, speed float64, voice, outPath string) error {
	return SynthesizeSpeaker(ctx, r, ttsText, speed, voice, 0, outPath)
}

// SynthesizeSpeaker is Synthesize for a voice that holds several speakers (see Speakers): speaker is
// the id Piper is asked for. Speaker 0, the default, adds no flag.
func SynthesizeSpeaker(ctx context.Context, r Runner, ttsText string, speed float64, voice string, speaker int, outPath string) error {
	if speaker < 0 {
		return fmt.Errorf("tts: speaker must be >= 0, got %d", speaker)
	}
	if speed <= 0 {
		return fmt.Errorf("tts: speed must be > 0, got %v", speed)
	}
	if voice == "" {
		return fmt.Errorf("tts: no voice given (set defaults.voice)")
	}

	lengthScale := 1.0 / speed

	args := []string{
		Flags.LengthScale, fmt.Sprintf("%g", lengthScale),
		Flags.Model, voice,
		Flags.OutputFile, StdoutTarget,
	}
	if speaker > 0 {
		args = append(args, Flags.Speaker, fmt.Sprintf("%d", speaker))
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("tts: open %s: %w", outPath, err)
	}
	_, runErr := r.Run(ctx, Binary, args, strings.NewReader(ttsText), f)
	closeErr := f.Close()
	if runErr != nil {
		_ = os.Remove(outPath)
		return runErr
	}
	if closeErr != nil {
		_ = os.Remove(outPath)
		return fmt.Errorf("tts: write %s: %w", outPath, closeErr)
	}
	if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
		_ = os.Remove(outPath)
		return fmt.Errorf("tts: piper produced no audio for %s", outPath)
	}
	return nil
}
