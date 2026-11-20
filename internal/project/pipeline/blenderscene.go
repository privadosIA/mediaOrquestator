package pipeline

// blenderscene.go: the scenes made from a .blend file (spec blender-engine). Blender renders the scene's frames on
// the card the run gives it (or the CPU), ffmpeg joins them into a video as long as the scene, and from there the
// scene is a clip like any other.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"mediaOrquestator/internal/platform/cache"
	"mediaOrquestator/internal/platform/events"
	"mediaOrquestator/internal/platform/runner"
	"mediaOrquestator/internal/project/config"
	"mediaOrquestator/internal/video/assemble"
	"mediaOrquestator/internal/video/blender"
)

// Seams for the tests, which have no Blender to hand.
var (
	detectBlender = blender.Cached
	renderBlender = blender.Render
	joinBlender   = blender.Join
)

// blenderRAMExtra is what Cycles needs on top of the size of the file: its memory is not predictable from the file
// alone, so a generous fixed amount is reserved.
const blenderRAMExtra = 2 << 30

// BlenderPath is where the video of a Blender scene is written, relative to the project.
func BlenderPath(sceneID string) string {
	return filepath.Join("tmp", fmt.Sprintf("scene_%s.blender.mp4", sceneID))
}

// BlenderFramesDir is where the frames of a Blender scene are written, relative to the project.
func BlenderFramesDir(sceneID string) string {
	return filepath.Join("tmp", fmt.Sprintf("scene_%s.blender", sceneID))
}

func blenderFile(scene config.Scene) string {
	return filepath.Join(config.SourcesSubdir, scene.Blender.File)
}

// BlenderImageHash is the image hash of a Blender scene: it follows what decides the render (see cache.BlenderParams)
// and nothing else, so an unchanged scene is not rendered again.
func BlenderImageHash(cfg *config.Config, scene config.Scene, base, audioHash string) string {
	b := scene.Blender
	length := audioHash
	if scene.IsSilent() {
		length = fmt.Sprintf("silence=%v", scene.SilenceDuration())
	}
	inject, _ := json.Marshal(b.Inject) // a map is encoded with its keys sorted
	version := "unavailable"
	if info, err := detectBlender(context.Background()); err == nil {
		version = info.Version
	}
	p := cache.BlenderParams{
		FileSHA256: fileSHA(joinDir(base, blenderFile(scene))),
		Inject:     string(inject),
		Samples:    b.ResolvedSamples(),
		Frames:     b.Frames,
		Resolution: cfg.Defaults.Resolution,
		FPS:        cfg.Defaults.FPS,
		Length:     length,
		Version:    version,
	}
	return cache.ImageHash(cache.ImageParams{Resolution: cfg.Defaults.Resolution, SourceImage: "blender:" + cache.BlenderHash(p)})
}

// fileSHA is the SHA-256 of a file's content, or "unreadable" when it cannot be read.
func fileSHA(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unreadable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(h.Sum(nil))
}

// blenderBytes is the RAM a Blender scene reserves: its file and a fixed margin.
func blenderBytes(base string, scene config.Scene) int64 {
	var size int64
	if info, err := os.Stat(joinDir(base, blenderFile(scene))); err == nil {
		size = info.Size()
	}
	return size + blenderRAMExtra
}

// gpuPinned says whether the run's environment gives the render a card: CUDA_VISIBLE_DEVICES set to something.
func gpuPinned(env []string) bool {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "CUDA_VISIBLE_DEVICES="); ok {
			return strings.TrimSpace(v) != ""
		}
	}
	return false
}

// renderBlenderScene renders a Blender scene to a video exactly as long as the scene and writes the video's first
// frame as the scene's picture.
func renderBlenderScene(ctx context.Context, r runner.CommandRunner, emitter *events.Emitter, base string, scene config.Scene, width, height, fps int, seconds float64, env []string, pngPath string) error {
	info, err := detectBlender(ctx)
	if err != nil {
		return err
	}
	b := scene.Blender
	frames := BlenderFramesDir(scene.ID)
	res, err := renderBlender(ctx, r, info.Path, blender.Params{
		File: blenderFile(scene), Dir: base, Out: frames, Inject: b.Inject, Samples: b.ResolvedSamples(),
		Device: b.ResolvedDevice(), GPU: gpuPinned(env), Width: width, Height: height, Frames: b.Frames,
		Count: int(math.Ceil(seconds * float64(fps))),
	}, env)
	if err != nil {
		return err
	}
	if res.Device == "CPU" && b.ResolvedDevice() == config.BlenderAuto {
		emitter.Warn(scene.ID, "Blender rendered on the CPU because the run gave it no GPU: it is slower")
	}
	out := BlenderPath(scene.ID)
	if err := joinBlender(ctx, r, base, frames, fps, seconds, out); err != nil {
		return err
	}
	if _, err := r.Run(ctx, "ffmpeg", assemble.BuildPosterArgs(out, width, height, 0, pngPath)); err != nil {
		return fmt.Errorf("scene %q: cannot take the first frame of the Blender render: %w", scene.ID, err)
	}
	_ = os.RemoveAll(joinDir(base, frames)) // the video is what is kept
	return nil
}
