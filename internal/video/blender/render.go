package blender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mediaOrquestator/internal/platform/runner"
)

// EnvVar is the environment variable the script reads its settings from, as JSON. The values to inject travel only
// here: they are never part of the script's text, so no text can break out of it.
const EnvVar = "MO_BLENDER"

// script is the whole of what runs inside Blender (passed inline, no file is written). It opens nothing itself: the
// file is the one on the command line, with auto-execution of its scripts disabled.
const script = `import bpy, os, sys, json, math

cfg = json.loads(os.environ['MO_BLENDER'])

def die(msg):
    print('MO_ERROR ' + msg, file=sys.stderr)
    sys.exit(1)

scene = bpy.context.scene
r = scene.render
r.engine = 'CYCLES'
r.resolution_x, r.resolution_y, r.resolution_percentage = cfg['width'], cfg['height'], 100
r.film_transparent = False
r.image_settings.file_format = 'PNG'
r.image_settings.color_mode = 'RGB'
scene.cycles.samples = cfg['samples']
scene.cycles.use_denoising = True
scene.cycles.seed = 0
scene.cycles.use_animated_seed = False

def srgb_to_linear(c):
    return c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4

def colour_of(val):
    if isinstance(val, str):
        h = val.lstrip('#')
        if len(h) != 6:
            die('colour %r: use #rrggbb' % val)
        return tuple(srgb_to_linear(int(h[i:i + 2], 16) / 255.0) for i in (0, 2, 4)) + (1.0,)
    if isinstance(val, list) and len(val) in (3, 4):
        v = [float(x) for x in val]
        return tuple(v) + ((1.0,) if len(v) == 3 else ())
    die('a colour is #rrggbb or a list of 3 or 4 numbers, got %r' % (val,))

def set_colour(mat, val):
    col = colour_of(val)
    if mat.use_nodes:
        for n in mat.node_tree.nodes:
            if n.type == 'BSDF_PRINCIPLED':
                n.inputs['Base Color'].default_value = col
                return
    mat.diffuse_color = col

for name, val in cfg['inject'].items():
    ob = bpy.data.objects.get(name)
    if ob is not None and ob.type == 'FONT':
        ob.data.body = str(val)
        continue
    mat = bpy.data.materials.get(name)
    if mat is not None:
        set_colour(mat, val)
        continue
    if '.' in name:
        obname, prop = name.rsplit('.', 1)
        ob = bpy.data.objects.get(obname)
        if ob is not None:
            ob[prop] = val
            continue
    die('inject: there is no text object, material or object property named %r in the file' % name)

if cfg.get('frames'):
    first, file_last = cfg['frames']
else:
    first, file_last = scene.frame_start, scene.frame_end
last = max(first, min(first + cfg['count'] - 1, file_last))
scene.frame_start, scene.frame_end = first, last

used = None
if cfg['device'] != 'cpu' and cfg['gpu']:
    prefs = bpy.context.preferences.addons['cycles'].preferences
    for t in ('OPTIX', 'CUDA'):
        try:
            prefs.compute_device_type = t
            prefs.get_devices()
        except Exception:
            continue
        devs = [d for d in prefs.devices if d.type == t]
        if devs:
            for d in prefs.devices:
                d.use = (d.type == t)
            scene.cycles.device = 'GPU'
            used = t
            if t == 'OPTIX':
                scene.cycles.denoiser = 'OPTIX'  # on the card: the CPU denoiser took 3/4 of the time
            break
if used is None:
    if cfg['device'] == 'gpu':
        die('device: gpu was asked for but Cycles sees no usable GPU (is a card given to the run, and is CUDA working?)')
    scene.cycles.device = 'CPU'
    used = 'CPU'
print('MO_DEVICE', used)
print('MO_RANGE', first, last)

r.filepath = os.path.join(cfg['out'], 'f_######')
r.use_file_extension = True
bpy.ops.render.render(animation=True)
`

// Params is what one render needs.
type Params struct {
	// File is the .blend to render; Dir is the project folder (the working directory) and Out the folder, inside it
	// or absolute, the PNG frames go to.
	File, Dir, Out string
	Inject         map[string]any
	Samples        int
	// Device is "auto", "gpu" or "cpu". GPU says whether a card is given to this render (CUDA_VISIBLE_DEVICES pinned
	// by the pipeline): without one, "auto" renders on the CPU and "gpu" fails.
	Device string
	GPU    bool
	Width  int
	Height int
	// Frames is [first, last] of the file's animation to use; nil means its own range.
	Frames []int
	// Count is how many frames the scene needs (its length times the frame rate).
	Count int
}

// Result says what the render did.
type Result struct {
	Device string // OPTIX, CUDA or CPU
	First  int
	Last   int
}

// Settings is the JSON the script reads from EnvVar.
func (p Params) Settings(out string) ([]byte, error) {
	inject := p.Inject
	if inject == nil {
		inject = map[string]any{}
	}
	device := p.Device
	if device == "" {
		device = "auto"
	}
	return json.Marshal(map[string]any{
		"inject": inject, "samples": p.Samples, "device": device, "gpu": p.GPU,
		"width": p.Width, "height": p.Height, "frames": p.Frames, "count": p.Count, "out": out,
	})
}

// BuildArgs are the arguments of the render. The script is always the same text; only the environment varies.
func BuildArgs(file string) []string {
	return []string{"--factory-startup", "-b", "-noaudio", file, "--disable-autoexec", "--python-exit-code", "1", "--python-expr", script}
}

// Script is the inline script, for the tests.
func Script() string { return script }

// Render writes the frames of the scene as PNG files into p.Out (emptied first) with Blender at bin. env holds the
// pins of the run (CUDA_VISIBLE_DEVICES=…); the settings are added to it.
func Render(ctx context.Context, r runner.CommandRunner, bin string, p Params, env []string) (Result, error) {
	if p.Count <= 0 {
		return Result{}, fmt.Errorf("blender: the scene has no frames to render")
	}
	out := p.Out
	if !filepath.IsAbs(out) {
		out = filepath.Join(p.Dir, out)
	}
	if err := os.RemoveAll(out); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return Result{}, err
	}
	settings, err := p.Settings(out)
	if err != nil {
		return Result{}, err
	}
	file := p.File
	if !filepath.IsAbs(file) {
		file = filepath.Join(p.Dir, file)
	}
	var stderr bytes.Buffer
	stdout, err := r.Run(ctx, bin, BuildArgs(file), runner.WithDir(p.Dir), runner.WithEnv(append(append([]string(nil), env...), EnvVar+"="+string(settings))...), runner.WithStderr(&stderr))
	if err != nil {
		return Result{}, fmt.Errorf("blender: %w", explain(err, stderr.String()))
	}
	res := parseResult(string(stdout))
	if n := countFrames(out); n == 0 {
		return res, fmt.Errorf("blender wrote no frames (%s)", tail(stderr.String(), string(stdout)))
	}
	return res, nil
}

// explain puts the script's own error line first, when it left one.
func explain(err error, stderr string) error {
	for _, line := range strings.Split(stderr, "\n") {
		if msg, ok := strings.CutPrefix(strings.TrimSpace(line), "MO_ERROR "); ok {
			return fmt.Errorf("%s", msg)
		}
	}
	return err
}

func tail(parts ...string) string {
	s := strings.TrimSpace(strings.Join(parts, " "))
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}

func parseResult(out string) Result {
	var res Result
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) == 2 && f[0] == "MO_DEVICE":
			res.Device = f[1]
		case len(f) == 3 && f[0] == "MO_RANGE":
			res.First, _ = strconv.Atoi(f[1])
			res.Last, _ = strconv.Atoi(f[2])
		}
	}
	return res
}

func countFrames(dir string) int {
	m, _ := filepath.Glob(filepath.Join(dir, "f_*.png"))
	return len(m)
}

// JoinArgs are the ffmpeg arguments that join the PNG frames of dir into a video of exactly seconds seconds: the last
// frame is held when the animation is shorter than the scene, and the video is cut when it is longer.
func JoinArgs(dir string, fps int, seconds float64, out string) []string {
	s := strconv.FormatFloat(seconds, 'f', 3, 64)
	return []string{
		"-y", "-framerate", strconv.Itoa(fps), "-pattern_type", "glob", "-i", filepath.Join(dir, "f_*.png"),
		"-vf", "tpad=stop_mode=clone:stop_duration=" + s + ",format=yuv420p",
		"-t", s, "-c:v", "libx264", "-preset", "fast", "-crf", "14", "-r", strconv.Itoa(fps), "-an", out,
	}
}

// Join joins the PNG frames written by Render into a video of exactly seconds seconds at fps, working in dir.
func Join(ctx context.Context, r runner.CommandRunner, dir, frames string, fps int, seconds float64, out string) error {
	if !filepath.IsAbs(frames) {
		frames = filepath.Join(dir, frames)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(dir, out)
	}
	if countFrames(frames) == 0 {
		return fmt.Errorf("blender: no frames in %s to join", frames)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if _, err := r.Run(ctx, "ffmpeg", JoinArgs(frames, fps, seconds, out), runner.WithDir(dir)); err != nil {
		return fmt.Errorf("blender: cannot join the frames: %w", err)
	}
	return nil
}
