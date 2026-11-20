package videoedit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVulkanDevices_TheCardOfARunIsLookedForByName(t *testing.T) {
	summary := `Devices:
GPU0:
	apiVersion         = 1.4.351
	deviceName         = NVIDIA GeForce RTX 4070
GPU1:
	deviceName         = NVIDIA GeForce RTX 5060 Ti
GPU2:
	deviceType         = PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU
	deviceName         = Intel(R) Graphics (ARL)
`
	devices := parseVulkanDevices(summary)
	if len(devices) != 3 || devices[1] != "NVIDIA GeForce RTX 5060 Ti" {
		t.Fatalf("devices = %q", devices)
	}
	for name, want := range map[string]int{"NVIDIA GeForce RTX 5060 Ti": 1, "NVIDIA GeForce RTX 4070": 0} {
		if i, ok := vulkanIndex(name, devices); !ok || i != want {
			t.Errorf("%s: %d %v, want %d", name, i, ok, want)
		}
	}
	// Vulkan lists the integrated card first on some machines: the index is not the CUDA one
	if i, ok := vulkanIndex("NVIDIA GeForce RTX 4070", []string{"Intel(R) Graphics", "NVIDIA GeForce RTX 4070"}); !ok || i != 1 {
		t.Errorf("reordered: %d %v", i, ok)
	}
	if _, ok := vulkanIndex("AMD Radeon", devices); ok {
		t.Error("a card that is not listed has no index")
	}
}

func withTools(c *Catalog, dir string) { c.ToolsDir = func() string { return dir } }

// fakeInstall puts the files the two programs are looked for by into dir/bin.
func fakeInstall(t *testing.T, c *Catalog, which ...string) string {
	t.Helper()
	root := t.TempDir()
	put := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range which {
		switch w {
		case "realesrgan":
			put(filepath.Join(root, "realesrgan", "realesrgan-ncnn-vulkan"))
			for _, x := range []string{"x2", "x3", "x4"} {
				put(filepath.Join(root, "realesrgan", "models", RealESRGANModel+"-"+x+".bin"))
			}
		case "rife":
			put(filepath.Join(root, "rife", "rife-ncnn-vulkan"))
			put(filepath.Join(root, "rife", RIFEModel, "flownet.bin"))
		}
	}
	withTools(c, root)
	return root
}

func TestEngineNotes_SayWhatIsNotInstalledAndHowToGetIt(t *testing.T) {
	c := NewCatalog()
	fakeInstall(t, c)
	for _, id := range []string{Scale, Interpolate} {
		notes := c.EngineNotes(id)
		if len(notes) != 1 || !strings.Contains(notes[0], "scripts/get-upscalers.sh") {
			t.Errorf("%s: %v", id, notes)
		}
	}
	if len(c.EngineNotes(Depth)) != 0 || len(c.EngineNotes(Reframe)) != 0 {
		t.Error("the other tools have no extra engines")
	}
	fakeInstall(t, c, "realesrgan", "rife")
	for _, id := range []string{Scale, Interpolate} {
		if notes := c.EngineNotes(id); len(notes) != 0 {
			t.Errorf("%s with everything installed: %v", id, notes)
		}
	}
	// a program without its model is not installed, and the note says which part is missing
	root := fakeInstall(t, c, "realesrgan")
	_ = os.Remove(filepath.Join(root, "realesrgan", "models", RealESRGANModel+"-x3.bin"))
	if ok, why := c.RealESRGANInstalled(); ok || !strings.Contains(why, "x3") {
		t.Errorf("%v %q", ok, why)
	}
}

// programStandIn plays the two programs: the enlarger scales every frame it is given with ffmpeg, and the interpolator
// writes the number of frames it is asked for, taking them in turn from the ones it is given.
type programStandIn struct {
	calls   [][]string
	fail    error
	short   bool // gives one frame fewer than it should
	skipOut bool
}

func (p *programStandIn) run(ctx context.Context, name string, args []string) ([]byte, error) {
	p.calls = append(p.calls, append([]string{filepath.Base(name)}, args...))
	if p.fail != nil {
		return nil, p.fail
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	opt := func(f string) string {
		for i, a := range args {
			if a == f && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	in, out := opt("-i"), opt("-o")
	frames, _ := filepath.Glob(filepath.Join(in, "*.png"))
	if p.skipOut {
		return nil, nil
	}
	switch filepath.Base(name) {
	case "realesrgan-ncnn-vulkan":
		factor := opt("-s")
		for _, f := range frames {
			b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", f, "-vf", "scale=iw*"+factor+":ih*"+factor, filepath.Join(out, filepath.Base(f))).CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("%w: %s", err, b)
			}
		}
		if p.short {
			_ = os.Remove(filepath.Join(out, filepath.Base(frames[0])))
		}
	case "rife-ncnn-vulkan":
		n, _ := strconv.Atoi(opt("-n"))
		if p.short {
			n--
		}
		for i := 0; i < n; i++ {
			src := frames[i*len(frames)/n]
			data, _ := os.ReadFile(src)
			if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("%08d.png", i+1)), data, 0o644); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

func withProgram(c *Catalog, p *programStandIn) { c.RunProgram = p.run }

func TestScale_Quality_UsesTheNetworkWhenItIsInstalled(t *testing.T) {
	c := NewCatalog()
	root := fakeInstall(t, c, "realesrgan")
	p := &programStandIn{}
	withProgram(c, p)
	src := movingClip(t, 160, 90, 16, 2, true)
	res, warned := runTool(t, c, Scale, src, map[string]any{"factor": 2, "engine": "quality"})
	if res.Engine != RealESRGANModel || len(res.Notes) != 0 || warned != "" {
		t.Errorf("res = %+v warned %q", res, warned)
	}
	if res.Width != 320 || res.Height != 180 || res.FPS != 16 || !res.HasAudio || !near(res.Duration, 2, 0.2) {
		t.Errorf("res = %+v: the size doubled and the rate, the length and the sound are the original's", res)
	}
	if len(p.calls) != 1 {
		t.Fatalf("calls = %v", p.calls)
	}
	call := strings.Join(p.calls[0], " ")
	for _, want := range []string{"-n " + RealESRGANModel, "-s 2", "-m " + filepath.Join(root, "realesrgan", "models"), "-f png"} {
		if !strings.Contains(call, want) {
			t.Errorf("the program was run as %q: lacks %q", call, want)
		}
	}
	// the frames of the work are not left in the result
	if frames := countPNG(filepath.Join(res.dir, "tmp", "scale-quality", "up")); frames < 30 {
		t.Errorf("the work frames: %d", frames)
	}
}

func TestScale_Quality_FallsBackToLanczosAndSaysWhy(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 160, 90, 16, 1, false)
	for name, tc := range map[string]struct {
		setup func(t *testing.T) *programStandIn
		want  string
	}{
		"not installed": {func(t *testing.T) *programStandIn { fakeInstall(t, c); return &programStandIn{} }, "not installed"},
		"the program fails": {func(t *testing.T) *programStandIn {
			fakeInstall(t, c, "realesrgan")
			return &programStandIn{fail: errors.New("vkCreateInstance failed")}
		}, "vkCreateInstance failed"},
		"a frame is lost": {func(t *testing.T) *programStandIn {
			fakeInstall(t, c, "realesrgan")
			return &programStandIn{short: true}
		}, "frames of the"},
		"no disk": {func(t *testing.T) *programStandIn {
			fakeInstall(t, c, "realesrgan")
			c.FreeBytes = func(string) uint64 { return 1000 }
			return &programStandIn{}
		}, "GB free"},
	} {
		t.Run(name, func(t *testing.T) {
			withProgram(c, tc.setup(t))
			res, warned := runTool(t, c, Scale, src, map[string]any{"factor": 2, "engine": "quality"})
			if res.Engine != "fast" || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], tc.want) || !strings.Contains(warned, tc.want) {
				t.Errorf("res = %+v warned %q, want a note with %q", res, warned, tc.want)
			}
			if res.Width != 320 || res.Height != 180 {
				t.Errorf("the fallback must still scale: %dx%d", res.Width, res.Height)
			}
		})
	}
}

func TestScale_Fast_NeverTouchesTheNetwork(t *testing.T) {
	c := NewCatalog()
	fakeInstall(t, c, "realesrgan")
	p := &programStandIn{}
	withProgram(c, p)
	res, _ := runTool(t, c, Scale, movingClip(t, 160, 90, 16, 1, false), map[string]any{"factor": 2})
	if res.Engine != "fast" || len(p.calls) != 0 || len(res.Notes) != 0 {
		t.Errorf("res = %+v calls %v", res, p.calls)
	}
}

func TestScale_Quality_ACancelledRunIsNotAFallback(t *testing.T) {
	c := NewCatalog()
	fakeInstall(t, c, "realesrgan")
	withProgram(c, &programStandIn{})
	src := movingClip(t, 160, 90, 16, 1, false)
	impl, _ := c.ImplOf(Scale)
	st, _ := ValidateSettings(Scale, map[string]any{"engine": "quality"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := t.TempDir()
	if _, err := impl.Run(ctx, Job{Dir: out, OutDir: out, Video: src, Settings: st, Warn: func(string) {}}); err == nil {
		t.Error("a cancelled run must stop, not make a lanczos result")
	}
}

func TestInterpolate_UsesTheNetworkAndKeepsTheLength(t *testing.T) {
	c := NewCatalog()
	root := fakeInstall(t, c, "rife")
	p := &programStandIn{}
	withProgram(c, p)
	res, warned := runTool(t, c, Interpolate, movingClip(t, 160, 90, 16, 2, true), map[string]any{"fps": 30})
	if res.Engine != RIFEModel || len(res.Notes) != 0 || warned != "" || res.FPS != 30 || !near(res.Duration, 2, 0.15) || res.HasAudio {
		t.Errorf("res = %+v warned %q", res, warned)
	}
	call := strings.Join(p.calls[0], " ")
	// 32 frames over 2 seconds make 60 at 30 per second
	for _, want := range []string{"-m " + filepath.Join(root, "rife", RIFEModel), "-n 60"} {
		if !strings.Contains(call, want) {
			t.Errorf("the program was run as %q: lacks %q", call, want)
		}
	}
}

func TestInterpolate_FallsBackToMinterpolateAndSaysWhy(t *testing.T) {
	c := NewCatalog()
	src := movingClip(t, 160, 90, 16, 1, false)
	for name, tc := range map[string]struct {
		setup func(t *testing.T) *programStandIn
		want  string
	}{
		"not installed": {func(t *testing.T) *programStandIn { fakeInstall(t, c); return &programStandIn{} }, "not installed"},
		"the program fails": {func(t *testing.T) *programStandIn {
			fakeInstall(t, c, "rife")
			return &programStandIn{fail: errors.New("no Vulkan device")}
		}, "no Vulkan device"},
		"a frame is lost": {func(t *testing.T) *programStandIn { fakeInstall(t, c, "rife"); return &programStandIn{short: true} }, "not the"},
	} {
		t.Run(name, func(t *testing.T) {
			withProgram(c, tc.setup(t))
			res, warned := runTool(t, c, Interpolate, src, map[string]any{"fps": 30})
			if res.Engine != "minterpolate" || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], tc.want) || !strings.Contains(warned, tc.want) || res.FPS != 30 {
				t.Errorf("res = %+v warned %q, want a note with %q", res, warned, tc.want)
			}
		})
	}
}

// With the real programs and the real card, on a small clip. Skipped where they are not installed.
func realToolsOrSkip(t *testing.T, c *Catalog, names ...string) {
	t.Helper()
	root := filepath.Join(repoRoot(t), "bin")
	c.ToolsDir = func() string { return root }
	for _, n := range names {
		var ok bool
		var why string
		if n == "realesrgan" {
			ok, why = c.RealESRGANInstalled()
		} else {
			ok, why = c.RIFEInstalled()
		}
		if !ok {
			t.Skipf("%s is not installed (%s)", n, why)
		}
	}
	if ok, _ := ffmpegAvailable(); !ok {
		t.Skip("ffmpeg is not installed")
	}
}

func TestScale_Quality_RealNetwork(t *testing.T) {
	c := NewCatalog()
	realToolsOrSkip(t, c, "realesrgan")
	src := movingClip(t, 208, 120, 16, 2, true)
	for _, factor := range []float64{2, 4} {
		res, warned := runTool(t, c, Scale, src, map[string]any{"factor": factor, "engine": "quality"})
		if res.Engine != RealESRGANModel || len(res.Notes) != 0 {
			t.Fatalf("x%g: res = %+v warned %q (the network was not used)", factor, res, warned)
		}
		if res.Width != int(208*factor) || res.Height != int(120*factor) || res.FPS != 16 || !res.HasAudio || !near(res.Duration, 2, 0.2) {
			t.Errorf("x%g: res = %+v", factor, res)
		}
	}
}

func TestInterpolate_RealNetwork(t *testing.T) {
	c := NewCatalog()
	realToolsOrSkip(t, c, "rife")
	src := movingClip(t, 320, 180, 16, 2, true)
	res, warned := runTool(t, c, Interpolate, src, map[string]any{"fps": 30})
	if res.Engine != RIFEModel || len(res.Notes) != 0 {
		t.Fatalf("res = %+v warned %q (the network was not used)", res, warned)
	}
	if res.FPS != 30 || !near(res.Duration, 2, 0.15) || res.Width != 320 || res.Height != 180 {
		t.Errorf("res = %+v", res)
	}
	if n := frameCount(t, filepath.Join(res.dir, res.File)); n < 59 || n > 61 {
		t.Errorf("frames = %d, want about 60", n)
	}
}
