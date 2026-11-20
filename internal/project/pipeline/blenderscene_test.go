package pipeline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaOrquestator/internal/platform/cache"
	"mediaOrquestator/internal/platform/events"
	"mediaOrquestator/internal/platform/runner"
	"mediaOrquestator/internal/project/config"
	"mediaOrquestator/internal/video/blender"
)

type blenderCall struct {
	Params blender.Params
	Env    []string
	Bin    string
}

type joinCall struct {
	Frames  string
	FPS     int
	Seconds float64
	Out     string
}

// blenderStubs replaces Blender: found as version 5.2.3, "rendering" is recorded and answers with the device given.
func blenderStubs(t *testing.T, device string) (*[]blenderCall, *[]joinCall) {
	t.Helper()
	renders, joins := &[]blenderCall{}, &[]joinCall{}
	oldD, oldR, oldJ := detectBlender, renderBlender, joinBlender
	detectBlender = func(context.Context) (blender.Info, error) {
		return blender.Info{Path: "/fake/blender", Version: "5.2.3"}, nil
	}
	renderBlender = func(ctx context.Context, r runner.CommandRunner, bin string, p blender.Params, env []string) (blender.Result, error) {
		*renders = append(*renders, blenderCall{Params: p, Env: env, Bin: bin})
		return blender.Result{Device: device, First: 1, Last: p.Count}, nil
	}
	joinBlender = func(ctx context.Context, r runner.CommandRunner, dir, frames string, fps int, seconds float64, out string) error {
		*joins = append(*joins, joinCall{frames, fps, seconds, out})
		return os.WriteFile(filepath.Join(dir, out), []byte("video"), 0o644)
	}
	t.Cleanup(func() { detectBlender, renderBlender, joinBlender = oldD, oldR, oldJ })
	return renders, joins
}

func blenderScene(id string) config.Scene {
	return config.Scene{ID: id, Blender: &config.SceneBlender{File: "turn.blend", Inject: map[string]any{"Title": "Hi"}}, SilenceSeconds: silent(2), Speed: 1.0}
}

func runBlenderScenes(t *testing.T, scenes []config.Scene, manifest *cache.Manifest, env []string) (*runner.FakeRunner, *cache.Manifest, string) {
	t.Helper()
	cfg := &config.Config{
		Project:  "demo",
		Output:   config.Output{File: "out/demo.mp4"},
		Defaults: config.Defaults{Model: "m.safetensors", Voice: "v.onnx", Resolution: "1280x720", Sampler: "euler", Steps: 30, FPS: 30},
		Scenes:   scenes,
	}
	fr := runner.NewFakeRunner()
	fr.SetHandler(func(inv runner.Invocation) runner.ScriptedResult {
		if inv.Name == "ffprobe" {
			return runner.ScriptedResult{Stdout: []byte(`{"format":{"duration":"3.0"}}`)}
		}
		return runner.ScriptedResult{}
	})
	var buf bytes.Buffer
	if manifest == nil {
		manifest = &cache.Manifest{Scenes: map[string]cache.SceneEntry{}}
	}
	if _, err := runVideo(context.Background(), cfg, events.New(&buf, true), fr, ttsRunnerAdapter{r: fr}, assembleRunnerAdapter{r: fr},
		manifest, cache.NewOnlyFilter(""), false, env, "tmp", "", nil, nil); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	return fr, manifest, buf.String()
}

func TestBlenderScene_RendersForTheSceneLengthWithoutSD(t *testing.T) {
	imageSetup(t)
	os.WriteFile(filepath.Join("sources", "turn.blend"), []byte("BLEND one"), 0o644)
	renders, joins := blenderStubs(t, "OPTIX")
	fr, _, out := runBlenderScenes(t, []config.Scene{blenderScene("a")}, nil, []string{"CUDA_VISIBLE_DEVICES=1"})
	for _, c := range fr.Calls() {
		if c.Name == "sd" || c.Name == "piper" {
			t.Fatalf("a silent Blender scene needs neither sd nor piper: %s", callNames(fr))
		}
	}
	if len(*renders) != 1 {
		t.Fatalf("one render, got %d", len(*renders))
	}
	r := (*renders)[0]
	p := r.Params
	if p.Count != 60 || p.Width != 1280 || p.Height != 720 || p.File != "sources/turn.blend" || p.Device != "auto" || !p.GPU || p.Samples != config.DefaultBlenderSamples || p.Inject["Title"] != "Hi" {
		t.Errorf("params = %+v", p)
	}
	if r.Bin != "/fake/blender" || len(r.Env) != 1 || r.Env[0] != "CUDA_VISIBLE_DEVICES=1" {
		t.Errorf("bin=%q env=%v: the card of the run must reach Blender", r.Bin, r.Env)
	}
	if len(*joins) != 1 || (*joins)[0].Seconds != 2 || (*joins)[0].FPS != 30 || (*joins)[0].Out != "tmp/scene_a.blender.mp4" {
		t.Errorf("joins = %+v", *joins)
	}
	clip := lastClipCall(t, fr, "a")
	if !strings.Contains(clip, "-stream_loop -1 -i tmp/scene_a.blender.mp4") || !strings.Contains(clip, "-t 2.000000") || !strings.Contains(clip, "setpts=1.000000*PTS") {
		t.Errorf("the clip takes the render at its own speed: %s", clip)
	}
	if strings.Contains(out, `"phase":"animate"`) || strings.Contains(out, "rendered on the CPU") {
		t.Errorf("no animate phase, and no CPU warning on a card:\n%s", out)
	}
}

func TestBlenderScene_NoCardMeansCPUAndSaysSo(t *testing.T) {
	imageSetup(t)
	os.WriteFile(filepath.Join("sources", "turn.blend"), []byte("x"), 0o644)
	renders, _ := blenderStubs(t, "CPU")
	_, _, out := runBlenderScenes(t, []config.Scene{blenderScene("a")}, nil, nil)
	if (*renders)[0].Params.GPU {
		t.Error("a run with no card must not tell Blender it has one")
	}
	if !strings.Contains(out, "rendered on the CPU") {
		t.Errorf("the log must say it was slower:\n%s", out)
	}
}

func TestBlenderScene_CachedUntilSomethingThatMattersChanges(t *testing.T) {
	imageSetup(t)
	blend := filepath.Join("sources", "turn.blend")
	os.WriteFile(blend, []byte("BLEND one"), 0o644)
	renders, _ := blenderStubs(t, "OPTIX")
	scenes := []config.Scene{blenderScene("a")}
	_, m, _ := runBlenderScenes(t, scenes, nil, []string{"CUDA_VISIBLE_DEVICES=0"})
	_, m, out := runBlenderScenes(t, scenes, m, []string{"CUDA_VISIBLE_DEVICES=1"}) // another card: same picture
	if len(*renders) != 1 || !strings.Contains(out, `"type":"skipped","scene_id":"a","phase":"render"`) {
		t.Fatalf("unchanged (even on another card) must be cached (%d renders):\n%s", len(*renders), out)
	}
	step := func(what string, change func()) {
		t.Helper()
		before := len(*renders)
		change()
		_, m, _ = runBlenderScenes(t, scenes, m, nil)
		if len(*renders) != before+1 {
			t.Errorf("%s must render again", what)
		}
	}
	step("a changed file", func() { os.WriteFile(blend, []byte("BLEND two"), 0o644) })
	step("a changed text", func() { scenes[0].Blender.Inject = map[string]any{"Title": "Bye"} })
	step("changed samples", func() { scenes[0].Blender.Samples = 8 })
	step("a longer scene", func() { scenes[0].SilenceSeconds = silent(3) })
}

func TestBlenderScene_NeedsNoDiffusionModel(t *testing.T) {
	cfg := &config.Config{Scenes: []config.Scene{blenderScene("a")}}
	if needsRender(cfg) {
		t.Error("a project of Blender scenes needs no diffusion program")
	}
	cfg.Scenes = append(cfg.Scenes, config.Scene{ID: "b", Prompt: "p"})
	if !needsRender(cfg) {
		t.Error("an ordinary scene still does")
	}
}

func TestBlenderScene_HashIgnoresTheDeviceAndFollowsTheVersion(t *testing.T) {
	imageSetup(t)
	os.WriteFile(filepath.Join("sources", "turn.blend"), []byte("x"), 0o644)
	blenderStubs(t, "OPTIX")
	cfg := &config.Config{Defaults: config.Defaults{Resolution: "1280x720", FPS: 30}}
	a, b := blenderScene("a"), blenderScene("a")
	b.Blender.Device = "cpu"
	if BlenderImageHash(cfg, a, "", "") != BlenderImageHash(cfg, b, "", "") {
		t.Error("the device is not part of the key")
	}
	h := BlenderImageHash(cfg, a, "", "")
	detectBlender = func(context.Context) (blender.Info, error) { return blender.Info{Version: "5.3.0"}, nil }
	if BlenderImageHash(cfg, a, "", "") == h {
		t.Error("another Blender version renders again")
	}
}
