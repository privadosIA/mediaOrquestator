package assemble

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  [][]string
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, args []string) (string, string, error) {
	f.calls = append(f.calls, args)
	return "", f.stderr, f.err
}

func TestBuildClipArgs_DefaultsFramerateWhenNonPositive(t *testing.T) {
	scene := Scene{ID: "s1", ImagePath: "img.png", AudioPath: "a.wav", Duration: 2.5}
	args := BuildClipArgs(scene, 0, "out.mp4")

	found := false
	for i, a := range args {
		if a == "-framerate" && i+1 < len(args) {
			found = true
			if args[i+1] != "30" {
				t.Fatalf("framerate = %q, want %q", args[i+1], "30")
			}
		}
	}
	if !found {
		t.Fatalf("args missing -framerate: %v", args)
	}
}

func TestBuildConcatListContent_EscapesSingleQuotes(t *testing.T) {
	content := BuildConcatListContent([]string{"tmp/scene_it's.mp4"})
	want := "file 'tmp/scene_it'\\''s.mp4'\n"
	if content != want {
		t.Fatalf("BuildConcatListContent() = %q, want %q", content, want)
	}
}

func TestGenerateClip_PropagatesErrorAndStderr(t *testing.T) {
	fr := &fakeRunner{stderr: "boom", err: errors.New("exit status 1")}
	scene := Scene{ID: "s1", ImagePath: "img.png", AudioPath: "a.wav", Duration: 1}

	stderr, err := GenerateClip(context.Background(), fr, scene, 30, "out.mp4")
	if err == nil {
		t.Fatal("GenerateClip() error = nil, want non-nil")
	}
	if stderr != "boom" {
		t.Fatalf("stderr = %q, want %q", stderr, "boom")
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(fr.calls))
	}
}

func TestConcatClips_Success(t *testing.T) {
	fr := &fakeRunner{}
	stderr, err := ConcatClips(context.Background(), fr, "list.txt", "out.mp4")
	if err != nil {
		t.Fatalf("ConcatClips() error = %v, want nil", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestWriteConcatList_WritesExpectedFile(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteConcatList(dir, []string{"a.mp4", "b.mp4"})
	if err != nil {
		t.Fatalf("WriteConcatList() error = %v, want nil", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read list file: %v", err)
	}
	if !strings.Contains(string(data), "file 'a.mp4'") || !strings.Contains(string(data), "file 'b.mp4'") {
		t.Fatalf("list content = %q, missing expected entries", data)
	}
}

func TestCleanupTmp_KeepTmpSkipsRemoval(t *testing.T) {
	dir := t.TempDir()
	if err := CleanupTmp(dir, nil, true); err != nil {
		t.Fatalf("CleanupTmp() error = %v, want nil", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("tmp dir should still exist: %v", err)
	}
}

func TestCleanupTmp_PreservesManifestReferencedArtifacts(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.mp4")
	drop := filepath.Join(dir, "drop.tmp")
	os.WriteFile(keep, []byte("x"), 0o644)
	os.WriteFile(drop, []byte("x"), 0o644)

	manifest := &CacheManifest{Artifacts: []string{keep}}
	if err := CleanupTmp(dir, manifest, false); err != nil {
		t.Fatalf("CleanupTmp() error = %v, want nil", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("referenced artifact should survive: %v", err)
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("unreferenced artifact should be removed, stat err = %v", err)
	}
}

func TestBuildVideoFilter_AStillOfAnotherSizeIsCoverCroppedFirst(t *testing.T) {
	s := Scene{ID: "s", Duration: 3, MotionType: "zoom_in", MotionIntensity: 0.5, Width: 1024, Height: 576, SourceWidth: 1280, SourceHeight: 704}
	got := BuildVideoFilter(s, 24)
	want := "scale=1024:576:force_original_aspect_ratio=increase,crop=1024:576,"
	if !strings.HasPrefix(got, want) {
		t.Errorf("filter = %q, want it to start with %q (cropped, never stretched)", got, want)
	}
}

func TestBuildVideoFilter_SameSizeLeavesTheFilterUnchanged(t *testing.T) {
	base := Scene{ID: "s", Duration: 3, MotionType: "zoom_in", MotionIntensity: 0.5, Width: 1024, Height: 576}
	sameSize := base
	sameSize.SourceWidth, sameSize.SourceHeight = 1024, 576
	if BuildVideoFilter(base, 24) != BuildVideoFilter(sameSize, 24) {
		t.Error("a picture of the project's size must not add a filter")
	}
	if strings.Contains(BuildVideoFilter(sameSize, 24), "force_original_aspect_ratio") {
		t.Error("a picture of the project's size must not add a cover-crop")
	}
}

func TestBuildVideoFilter_ExplicitSpeedReplacesTheAutomaticFit(t *testing.T) {
	scene := Scene{AnimatedPath: "a.mp4", AnimatedDuration: 2, Duration: 3, Width: 640, Height: 360}
	if got := BuildVideoFilter(scene, 24); !strings.Contains(got, "setpts=1.500000*PTS") {
		t.Fatalf("automatic: a longer scene stretches the clip: %s", got)
	}
	scene.AnimatedSpeed = 0.5
	if got := BuildVideoFilter(scene, 24); !strings.Contains(got, "setpts=2.000000*PTS") {
		t.Fatalf("half speed doubles the timestamps: %s", got)
	}
	scene.AnimatedSpeed = 2
	if got := BuildVideoFilter(scene, 24); !strings.Contains(got, "setpts=0.500000*PTS") {
		t.Fatalf("double speed halves them, and the scene's length does not matter: %s", got)
	}
}

func TestBuildClipArgs_AHeldClipPlaysOnceAndKeepsItsLastFrame(t *testing.T) {
	s := Scene{ID: "s", AnimatedPath: "tmp/a.mp4", AudioPath: "a.wav", Duration: 8, AnimatedDuration: 4, AnimatedHold: true, Width: 1280, Height: 720}
	args := strings.Join(BuildClipArgs(s, 30, "out.mp4"), " ")
	if strings.Contains(args, "-stream_loop") || !strings.Contains(args, "tpad=stop_mode=clone") {
		t.Errorf("hold: %s", args)
	}
	s.AnimatedHold = false
	args = strings.Join(BuildClipArgs(s, 30, "out.mp4"), " ")
	if !strings.Contains(args, "-stream_loop -1") || strings.Contains(args, "tpad") {
		t.Errorf("loop: %s", args)
	}
}
