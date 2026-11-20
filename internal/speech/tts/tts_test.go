package tts

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner implementa tts.Runner sin lanzar procesos: guarda la invocación,
// escribe `out` en stdout y devuelve `stderr`/`err` tal cual.
type fakeRunner struct {
	calls  int
	name   string
	args   []string
	stdin  string
	out    []byte
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, stdin io.Reader, stdout io.Writer) (string, error) {
	f.calls++
	f.name, f.args = name, args
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	if f.out != nil {
		if _, err := stdout.Write(f.out); err != nil {
			return "", err
		}
	}
	return f.stderr, f.err
}

func TestSynthesizeSuccess(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "scene.wav")
	fr := &fakeRunner{out: []byte("RIFF")}

	if err := Synthesize(context.Background(), fr, "hola mundo", 2.0, "voices/v.onnx", outPath); err != nil {
		t.Fatalf("Synthesize() error = %v, want nil", err)
	}

	if fr.name != Binary {
		t.Fatalf("binary = %q, want %q", fr.name, Binary)
	}
	wantArgs := []string{Flags.LengthScale, "0.5", Flags.Model, "voices/v.onnx", Flags.OutputFile, "-"}
	if len(fr.args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", fr.args, wantArgs)
	}
	for i := range wantArgs {
		if fr.args[i] != wantArgs[i] {
			t.Fatalf("args[%d] = %q, want %q", i, fr.args[i], wantArgs[i])
		}
	}
	if fr.stdin != "hola mundo" {
		t.Fatalf("stdin = %q, want %q", fr.stdin, "hola mundo")
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("leer %s: %v", outPath, err)
	}
	if string(got) != "RIFF" {
		t.Fatalf("contenido del .wav = %q, want %q", got, "RIFF")
	}
}

func TestSynthesizeRejectsNonPositiveSpeed(t *testing.T) {
	for _, speed := range []float64{0, -1} {
		fr := &fakeRunner{}
		err := Synthesize(context.Background(), fr, "hola", speed, "v.onnx", filepath.Join(t.TempDir(), "x.wav"))
		if err == nil {
			t.Fatalf("speed %v: error = nil, want non-nil", speed)
		}
		if fr.calls != 0 {
			t.Fatalf("speed %v: Piper invocado %d veces, want 0", speed, fr.calls)
		}
	}
}

func TestSynthesizePropagatesError(t *testing.T) {
	wantErr := errors.New("exit status 1")
	fr := &fakeRunner{stderr: "boom", err: wantErr}

	err := Synthesize(context.Background(), fr, "hola", 1.0, "v.onnx", filepath.Join(t.TempDir(), "x.wav"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Synthesize() error = %v, want %v", err, wantErr)
	}
}

func TestSynthesizeNeverPassesAFilePathToPiper(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "deep", "scene.wav")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{out: []byte("RIFF")}
	if err := Synthesize(context.Background(), fr, "x", 1, "v.onnx", outPath); err != nil {
		t.Fatal(err)
	}
	for _, a := range fr.args {
		if a == outPath || filepath.Base(a) == "scene.wav" {
			t.Fatalf("output path leaked into Piper args: %v", fr.args)
		}
	}
}

func TestSynthesizeRejectsEmptyVoice(t *testing.T) {
	fr := &fakeRunner{out: []byte("RIFF")}
	out := filepath.Join(t.TempDir(), "x.wav")
	if err := Synthesize(context.Background(), fr, "hola", 1, "", out); err == nil {
		t.Fatal("empty voice: error = nil, want non-nil")
	}
	if fr.calls != 0 {
		t.Fatalf("Piper invocado %d veces, want 0", fr.calls)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("no file may be created when the voice is empty")
	}
}

func TestSynthesizeEmptyOutputIsAnErrorAndLeavesNoFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.wav")
	fr := &fakeRunner{} // succeeds, emits nothing
	err := Synthesize(context.Background(), fr, "hola", 1, "v.onnx", out)
	if err == nil || !strings.Contains(err.Error(), "no audio") {
		t.Fatalf("error = %v, want a no-audio error", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("the empty file must be removed")
	}
}

func TestSynthesizeRunnerErrorRemovesPartialFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.wav")
	fr := &fakeRunner{out: []byte("RI"), err: errors.New("exit status 1")}
	if err := Synthesize(context.Background(), fr, "hola", 1, "v.onnx", out); err == nil {
		t.Fatal("want error")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("a failed synthesis must not leave a partial WAV")
	}
}

func TestSynthesizeSpeaker(t *testing.T) {
	fr := &fakeRunner{out: []byte("RIFF")}
	if err := SynthesizeSpeaker(context.Background(), fr, "hola", 1, "v.onnx", 1, filepath.Join(t.TempDir(), "a.wav")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fr.args, " "); !strings.HasSuffix(got, "--speaker 1") {
		t.Errorf("args = %q, want them to end with --speaker 1", got)
	}
	// Speaker 0 adds no flag: the arguments are those of the plain voice.
	fr = &fakeRunner{out: []byte("RIFF")}
	if err := SynthesizeSpeaker(context.Background(), fr, "hola", 1, "v.onnx", 0, filepath.Join(t.TempDir(), "b.wav")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(fr.args, " "), "--speaker") {
		t.Errorf("args = %v, want no --speaker for speaker 0", fr.args)
	}
	if err := SynthesizeSpeaker(context.Background(), fr, "hola", 1, "v.onnx", -1, filepath.Join(t.TempDir(), "c.wav")); err == nil {
		t.Error("a negative speaker must be refused")
	}
}

func TestSpeakers(t *testing.T) {
	dir := t.TempDir()
	write := func(name, json string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p+".json", []byte(json), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	two := write("two.onnx", `{"num_speakers": 2, "speaker_id_map": {"M": 0, "F": 1}}`)
	got := Speakers(two)
	if len(got) != 2 || got[0] != (Speaker{0, "M"}) || got[1] != (Speaker{1, "F"}) {
		t.Errorf("Speakers = %v", got)
	}
	unnamed := write("unnamed.onnx", `{"num_speakers": 3}`)
	if got := Speakers(unnamed); len(got) != 3 || got[2].Name != "2" {
		t.Errorf("unnamed speakers = %v, want the ids as names", got)
	}
	if got := Speakers(write("one.onnx", `{"num_speakers": 1}`)); got != nil {
		t.Errorf("a single-speaker voice has nothing to choose, got %v", got)
	}
	if got := Speakers(filepath.Join(dir, "missing.onnx")); got != nil || SpeakerCount(filepath.Join(dir, "missing.onnx")) != 1 {
		t.Errorf("a voice without a readable config counts as one speaker")
	}
	if SpeakerCount(two) != 2 {
		t.Errorf("SpeakerCount = %d, want 2", SpeakerCount(two))
	}
}
