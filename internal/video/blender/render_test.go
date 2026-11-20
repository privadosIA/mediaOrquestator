package blender

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaOrquestator/internal/platform/runner"
)

// fakeRender makes the fake Blender write n frames into the folder it was told and report a device.
func fakeRender(t *testing.T, n int, device string) *runner.FakeRunner {
	t.Helper()
	fr := runner.NewFakeRunner()
	fr.SetHandler(func(inv runner.Invocation) runner.ScriptedResult {
		var cfg map[string]any
		for _, e := range inv.Env {
			if v, ok := strings.CutPrefix(e, EnvVar+"="); ok {
				if err := json.Unmarshal([]byte(v), &cfg); err != nil {
					t.Errorf("the settings are not JSON: %v", err)
				}
			}
		}
		dir, _ := cfg["out"].(string)
		for i := 1; i <= n; i++ {
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("f_%06d.png", i)), []byte("png"), 0o644)
		}
		return runner.ScriptedResult{Stdout: []byte("Blender\nMO_DEVICE " + device + "\nMO_RANGE 1 " + fmt.Sprint(n) + "\n")}
	})
	return fr
}

func TestRender_TextTravelsInTheEnvironmentNeverInTheScript(t *testing.T) {
	dir := t.TempDir()
	awkward := "He said \"hi\"\\n\nline 2 ' ''' \u00e9"
	fr := fakeRender(t, 3, "CPU")
	_, err := Render(context.Background(), fr, "/usr/bin/blender", Params{
		File: "turn.blend", Dir: dir, Out: "tmp/f", Inject: map[string]any{"Title": awkward},
		Samples: 8, Device: "auto", Width: 160, Height: 90, Count: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := fr.Calls()[0]
	if strings.Contains(strings.Join(c.Args, "\x00"), "He said") {
		t.Fatal("the text must not be in the arguments")
	}
	if c.Args[len(c.Args)-1] != Script() {
		t.Error("the script must be exactly the fixed one")
	}
	var cfg struct {
		Inject map[string]any `json:"inject"`
	}
	for _, e := range c.Env {
		if v, ok := strings.CutPrefix(e, EnvVar+"="); ok {
			json.Unmarshal([]byte(v), &cfg)
		}
	}
	if cfg.Inject["Title"] != awkward {
		t.Errorf("the text arrived as %q", cfg.Inject["Title"])
	}
}

func TestRender_ArgumentsDisableAutoexecAndAreScriptless(t *testing.T) {
	args := BuildArgs("/p/turn.blend")
	joined := strings.Join(args[:len(args)-1], " ")
	for _, want := range []string{"--factory-startup", "-b", "/p/turn.blend", "--disable-autoexec", "--python-exit-code 1", "--python-expr"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, ".py") {
		t.Error("no Python file is involved")
	}
}

func TestRender_PassesThePinsAndReportsTheDevice(t *testing.T) {
	dir := t.TempDir()
	fr := fakeRender(t, 2, "OPTIX")
	res, err := Render(context.Background(), fr, "blender", Params{File: "a.blend", Dir: dir, Out: "f", GPU: true, Width: 64, Height: 64, Count: 2}, []string{"CUDA_VISIBLE_DEVICES=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Device != "OPTIX" || res.First != 1 || res.Last != 2 {
		t.Errorf("result = %+v", res)
	}
	c := fr.Calls()[0]
	if c.Env[0] != "CUDA_VISIBLE_DEVICES=1" || c.Dir != dir {
		t.Errorf("env=%v dir=%q", c.Env, c.Dir)
	}
}

func TestRender_EmptiesTheFramesFolderFirst(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "f"), 0o755)
	stale := filepath.Join(dir, "f", "f_000099.png")
	os.WriteFile(stale, []byte("old"), 0o644)
	if _, err := Render(context.Background(), fakeRender(t, 1, "CPU"), "blender", Params{File: "a.blend", Dir: dir, Out: "f", Width: 8, Height: 8, Count: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a frame of an earlier render was kept")
	}
}

func TestRender_ShowsTheScriptsOwnError(t *testing.T) {
	fr := runner.NewFakeRunner()
	fr.Script(runner.ScriptedResult{Err: fmt.Errorf("exit status 1 (stderr: noise\nMO_ERROR inject: there is no text object, material or object property named 'Nope' in the file\n)")})
	// the fake returns the error as a whole: explain() works on the captured stderr, so check the fallback keeps it
	_, err := Render(context.Background(), fr, "blender", Params{File: "a.blend", Dir: t.TempDir(), Out: "f", Width: 8, Height: 8, Count: 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("err = %v", err)
	}
}

func TestExplainPrefersTheScriptsLine(t *testing.T) {
	err := explain(fmt.Errorf("exit status 1"), "x\nMO_ERROR device: gpu was asked for\ny")
	if err.Error() != "device: gpu was asked for" {
		t.Errorf("err = %v", err)
	}
}

func TestRender_NoFramesIsAnError(t *testing.T) {
	fr := runner.NewFakeRunner()
	fr.Script(runner.ScriptedResult{Stdout: []byte("MO_DEVICE CPU\n")})
	if _, err := Render(context.Background(), fr, "blender", Params{File: "a.blend", Dir: t.TempDir(), Out: "f", Width: 8, Height: 8, Count: 1}, nil); err == nil || !strings.Contains(err.Error(), "no frames") {
		t.Errorf("err = %v", err)
	}
	if _, err := Render(context.Background(), fr, "blender", Params{File: "a.blend", Dir: t.TempDir(), Out: "f"}, nil); err == nil {
		t.Error("a scene with no frames to render is an error")
	}
}

func TestJoinArgsHoldsAndCutsToTheSceneLength(t *testing.T) {
	a := strings.Join(JoinArgs("/p/f", 24, 5, "/p/out.mp4"), " ")
	for _, want := range []string{"-framerate 24", "-pattern_type glob", "/p/f/f_*.png", "tpad=stop_mode=clone:stop_duration=5.000", "-t 5.000", "/p/out.mp4"} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in %q", want, a)
		}
	}
}

func TestSettingsDefaults(t *testing.T) {
	data, _ := Params{Width: 1, Height: 1, Count: 1}.Settings("/o")
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["device"] != "auto" || m["gpu"] != false || m["out"] != "/o" {
		t.Errorf("settings = %v", m)
	}
	if _, ok := m["inject"].(map[string]any); !ok {
		t.Errorf("inject must be an object, got %T", m["inject"])
	}
}
