package blender

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mediaOrquestator/internal/platform/runner"
)

// Real Blender: skipped unless MO_REAL_BLENDER=1 (it needs Blender installed).
func realBlender(t *testing.T) (Info, runner.CommandRunner) {
	t.Helper()
	if os.Getenv("MO_REAL_BLENDER") != "1" {
		t.Skip("set MO_REAL_BLENDER=1 to run against the installed Blender")
	}
	if _, err := exec.LookPath("blender"); err != nil {
		t.Skip("no blender")
	}
	r := runner.NewExecRunner()
	info, err := Detect(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return info, r
}

func TestRealDetectAndDevices(t *testing.T) {
	info, r := realBlender(t)
	t.Logf("blender %s at %s", info.Version, info.Path)
	devs, err := Devices(context.Background(), r, info.Path, "CUDA_VISIBLE_DEVICES=")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Type != "CPU" {
		t.Errorf("with the cards hidden only the CPU must be left: %+v", devs)
	}
	devs, err = Devices(context.Background(), r, info.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("devices: %+v", devs)
}

// makeBlend writes a small animated scene: a text object "Title", a sphere with the material "Ball" and a camera.
func makeBlend(t *testing.T, r runner.CommandRunner, bin, path string, frames int) {
	t.Helper()
	py := fmt.Sprintf(`import bpy, math
bpy.ops.wm.read_factory_settings(use_empty=True)
s = bpy.context.scene
s.frame_start, s.frame_end = 1, %d
bpy.ops.mesh.primitive_uv_sphere_add(radius=1, location=(0, 0, 1))
sp = bpy.context.object
m = bpy.data.materials.new('Ball'); m.use_nodes = True
sp.data.materials.append(m)
sp.keyframe_insert('location', frame=1); sp.location.z = 2; sp.keyframe_insert('location', frame=%d)
bpy.ops.object.text_add(location=(-1, 0, 0)); bpy.context.object.name = 'Title'; bpy.context.object.data.body = 'x'
bpy.ops.object.light_add(type='SUN', location=(2, -3, 5))
bpy.ops.object.camera_add(location=(0, -7, 2), rotation=(math.radians(80), 0, 0)); s.camera = bpy.context.object
bpy.ops.wm.save_as_mainfile(filepath=%q)
`, frames, frames, path)
	if _, err := r.Run(context.Background(), bin, []string{"--factory-startup", "-b", "-noaudio", "--python-exit-code", "1", "--python-expr", py}); err != nil {
		t.Fatal(err)
	}
}

func TestRealRenderInjectsAndCuts(t *testing.T) {
	info, r := realBlender(t)
	dir := t.TempDir()
	blend := filepath.Join(dir, "scene.blend")
	makeBlend(t, r, info.Path, blend, 4)
	render := func(out string, inject map[string]any, device string, gpu bool, count int) Result {
		res, err := Render(context.Background(), r, info.Path, Params{
			File: blend, Dir: dir, Out: out, Inject: inject, Samples: 4, Device: device, GPU: gpu, Width: 160, Height: 90, Count: count,
		}, nil)
		if err != nil {
			t.Fatalf("%s: %v", out, err)
		}
		return res
	}
	awkward := "He said \"hi\"\\n 'x'\nline 2"
	red := render("red", map[string]any{"Title": awkward, "Ball": "#ff0000"}, "cpu", false, 10)
	if red.Device != "CPU" || red.First != 1 || red.Last != 4 {
		t.Errorf("a 4-frame file asked for 10: the range is the file's, got %+v", red)
	}
	if n := countFrames(filepath.Join(dir, "red")); n != 4 {
		t.Errorf("frames = %d, want 4", n)
	}
	render("blue", map[string]any{"Ball": "#0000ff"}, "cpu", false, 2)
	a, _ := os.ReadFile(filepath.Join(dir, "red", "f_000001.png"))
	b, _ := os.ReadFile(filepath.Join(dir, "blue", "f_000001.png"))
	if string(a) == string(b) {
		t.Error("another colour must give another picture")
	}
	if _, err := Render(context.Background(), r, info.Path, Params{File: blend, Dir: dir, Out: "bad", Inject: map[string]any{"Nope": "x"}, Samples: 4, Width: 16, Height: 16, Count: 1}, nil); err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Errorf("an unknown name must be refused naming it: %v", err)
	}
	if _, err := Render(context.Background(), r, info.Path, Params{File: blend, Dir: dir, Out: "gpu", Device: "gpu", GPU: false, Samples: 4, Width: 16, Height: 16, Count: 1}, []string{"CUDA_VISIBLE_DEVICES="}); err == nil || !strings.Contains(err.Error(), "no usable GPU") {
		t.Errorf("device gpu without a card: %v", err)
	}
}

func TestRealRenderOnOneGPU(t *testing.T) {
	info, r := realBlender(t)
	dir := t.TempDir()
	blend := filepath.Join(dir, "scene.blend")
	makeBlend(t, r, info.Path, blend, 3)
	res, err := Render(context.Background(), r, info.Path, Params{File: blend, Dir: dir, Out: "g", Samples: 4, Device: "auto", GPU: true, Width: 160, Height: 90, Count: 3}, []string{"CUDA_VISIBLE_DEVICES=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Device != "OPTIX" && res.Device != "CUDA" {
		t.Errorf("with a card given the render must use it, got %q", res.Device)
	}
	devs, _ := Devices(context.Background(), r, info.Path, "CUDA_VISIBLE_DEVICES=1")
	gpus := 0
	for _, d := range devs {
		if d.Type == "OPTIX" {
			gpus++
		}
	}
	if gpus != 1 {
		t.Errorf("with one card pinned Cycles must see exactly one: %+v", devs)
	}
}

func TestRealJoinHoldsTheLastFrameAndCuts(t *testing.T) {
	info, r := realBlender(t)
	dir := t.TempDir()
	blend := filepath.Join(dir, "scene.blend")
	makeBlend(t, r, info.Path, blend, 4) // 4 frames of animation
	if _, err := Render(context.Background(), r, info.Path, Params{File: blend, Dir: dir, Out: "f", Samples: 2, Device: "cpu", Width: 160, Height: 90, Count: 24}, nil); err != nil {
		t.Fatal(err)
	}
	duration := func(out string) float64 {
		b, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", out).Output()
		if err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return float64(n) / 24
	}
	for _, secs := range []float64{3, 0.1} { // longer than the 4 frames: held; shorter: cut
		out := filepath.Join(dir, fmt.Sprintf("o%v.mp4", secs))
		if err := Join(context.Background(), runner.NewExecRunner(), dir, "f", 24, secs, out); err != nil {
			t.Fatal(err)
		}
		if got := duration(out); got < secs-0.05 || got > secs+0.05 {
			t.Errorf("a scene of %v s gave %v s", secs, got)
		}
	}
}

func TestRealAFileWithAnAutoRunScriptDoesNotRunIt(t *testing.T) {
	info, r := realBlender(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	blend := filepath.Join(dir, "boom.blend")
	py := fmt.Sprintf(`import bpy
bpy.ops.wm.read_factory_settings(use_empty=True)
tx = bpy.data.texts.new('boom.py')
tx.write("open('%s', 'w').write('x')")
tx.use_module = True
bpy.ops.object.camera_add(); bpy.context.scene.camera = bpy.context.object
bpy.ops.wm.save_as_mainfile(filepath=%q)
`, marker, blend)
	if _, err := r.Run(context.Background(), info.Path, []string{"--factory-startup", "-b", "-noaudio", "--python-exit-code", "1", "--python-expr", py}); err != nil {
		t.Fatal(err)
	}
	os.Remove(marker)
	// control: opened without the flag, the script runs (when this Blender allows auto-run at all)
	r.Run(context.Background(), info.Path, []string{"--factory-startup", "-b", "-noaudio", blend, "--python-expr", "pass"})
	_, controlRan := os.Stat(marker)
	os.Remove(marker)
	if _, err := Render(context.Background(), r, info.Path, Params{File: blend, Dir: dir, Out: "f", Samples: 1, Device: "cpu", Width: 16, Height: 16, Count: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the script of the file ran during the render")
	}
	if controlRan != nil {
		t.Log("note: this Blender does not auto-run scripts even without the flag, so the check is weak here")
	}
}
