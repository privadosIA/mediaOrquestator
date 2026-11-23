package blender

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPreviewFrame_IsTheFrameTheClipWouldHaveAtThatSecond(t *testing.T) {
	for _, tc := range []struct {
		t, seconds float64
		fps, want  int
	}{{0, 5, 30, 1}, {1, 5, 30, 31}, {0.5, 5, 30, 16}, {5, 5, 30, 150}, {4.99, 5, 30, 150}, {2, 5, 24, 49}, {0.01, 5, 30, 1}} {
		if got := PreviewFrame(tc.t, tc.seconds, tc.fps); got != tc.want {
			t.Errorf("t=%v of %vs at %dfps: frame %d, want %d", tc.t, tc.seconds, tc.fps, got, tc.want)
		}
	}
}

func TestPreview_OneSmallFrameOnTheCPU(t *testing.T) {
	spec, err := Resolve([]byte(minimal), testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		width, wantW, wantH int
	}{"the default": {0, 480, 270}, "asked": {600, 600, 338}, "too wide": {2000, 960, 540}} {
		fr := fakeRender(t, 150, "CPU")
		png, err := Preview(context.Background(), fr, "blender", spec, PreviewOptions{T: 1, Seconds: 5, FPS: 30, Width: tc.width, AspectW: 1280, AspectH: 720})
		if err != nil || string(png) != "png" {
			t.Fatalf("%s: %q %v", name, png, err)
		}
		c := fr.Calls()[0]
		var cfg struct {
			Frame, Count, Samples, Width, Height int
			Device                               string
			GPU                                  bool
		}
		var cudaHidden bool
		for _, e := range c.Env {
			if v, ok := strings.CutPrefix(e, EnvVar+"="); ok {
				json.Unmarshal([]byte(v), &cfg)
			}
			if e == "CUDA_VISIBLE_DEVICES=" {
				cudaHidden = true
			}
		}
		if cfg.Frame != 31 || cfg.Count != 150 || cfg.Samples != 8 || cfg.Width != tc.wantW || cfg.Height != tc.wantH || cfg.Device != "cpu" || cfg.GPU || !cudaHidden {
			t.Errorf("%s: settings = %+v, cards hidden %v", name, cfg, cudaHidden)
		}
	}
	for name, o := range map[string]PreviewOptions{
		"after the end": {T: 5.5, Seconds: 5, FPS: 30},
		"before":        {T: -1, Seconds: 5, FPS: 30},
		"no fps":        {T: 1, Seconds: 5},
	} {
		if _, err := Preview(context.Background(), fakeRender(t, 150, "CPU"), "blender", spec, o); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestScript_AFrameRendersOnlyThatFrameOfTheWholeScene(t *testing.T) {
	if !strings.Contains(script, "if cfg.get('build') and cfg.get('frame'):") || !strings.Contains(script, "first, file_last = cfg['frame'], cfg['frame']") {
		t.Error("the script must render one frame when it is given one")
	}
	spec, _ := Resolve([]byte(minimal), testEnv(""))
	data, _ := Params{Spec: spec, FPS: 30, Frame: 31, Width: 1, Height: 1, Count: 150}.Settings("/o")
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["frame"] != float64(31) || m["count"] != float64(150) {
		t.Errorf("settings = %v", m)
	}
	data, _ = Params{Spec: spec, FPS: 30, Width: 1, Height: 1, Count: 150}.Settings("/o")
	m = nil
	json.Unmarshal(data, &m)
	if _, ok := m["frame"]; ok {
		t.Error("without a frame the whole scene is rendered")
	}
}
