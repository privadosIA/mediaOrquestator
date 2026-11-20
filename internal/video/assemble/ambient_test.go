package assemble

import (
	"strings"
	"testing"
)

func ambientScene() Scene {
	return Scene{ID: "s1", ImagePath: "i.png", AudioPath: "n.wav", Duration: 5, AnimatedPath: "a.mp4", AnimatedDuration: 4,
		Width: 768, Height: 416, AmbientPath: "tmp/anim_s1.ambient.wav"}
}

func TestBuildClipArgs_AmbientNarrated(t *testing.T) {
	args := strings.Join(BuildClipArgs(ambientScene(), 30, "o.mp4"), " ")
	for _, want := range []string{"-i n.wav -i tmp/anim_s1.ambient.wav", "[2:a]", "volume=-18dB", "sidechaincompress", "amix=inputs=2", "-map 0:v -map [a]", "-vf "} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
	for _, want := range []string{"-c:a aac", "-b:a 192k", "-ar 44100", "-ac 2"} {
		if !strings.Contains(args, want) {
			t.Errorf("stream parameters must stay: missing %q", want)
		}
	}
}

func TestBuildClipArgs_AmbientSilentSceneHasNoDucking(t *testing.T) {
	s := ambientScene()
	s.Silent = true
	args := strings.Join(BuildClipArgs(s, 30, "o.mp4"), " ")
	if strings.Contains(args, "sidechaincompress") || !strings.Contains(args, "volume=-18dB") || !strings.Contains(args, "amix=inputs=2") {
		t.Errorf("args = %s", args)
	}
	if !strings.Contains(args, "anullsrc") {
		t.Errorf("the silent source must stay input 1: %s", args)
	}
}

func TestBuildClipArgs_AmbientWithOverlayIsTheLastInput(t *testing.T) {
	s := ambientScene()
	s.OverlayPath, s.OverlayStart, s.OverlayEnd = "ov.webm", 1, 3
	args := strings.Join(BuildClipArgs(s, 30, "o.mp4"), " ")
	if !strings.Contains(args, "-i ov.webm -i tmp/anim_s1.ambient.wav") || !strings.Contains(args, "[3:a]") || !strings.Contains(args, "-map [v] -map [a]") {
		t.Errorf("args = %s", args)
	}
}

func TestBuildClipArgs_WithoutAmbientIsUnchanged(t *testing.T) {
	s := ambientScene()
	s.AmbientPath = ""
	args := strings.Join(BuildClipArgs(s, 30, "o.mp4"), " ")
	if strings.Contains(args, "filter_complex") || strings.Contains(args, "amix") || strings.Contains(args, "ambient") {
		t.Errorf("a scene without ambience must keep its arguments: %s", args)
	}
}

func TestBuildClipArgs_AmbientSpeechIsMixedAtFullLevel(t *testing.T) {
	s := ambientScene()
	s.Silent = true
	s.AmbientSpeech = true
	args := strings.Join(BuildClipArgs(s, 30, "o.mp4"), " ")
	if !strings.Contains(args, "volume=0dB") || strings.Contains(args, "volume=-18dB") {
		t.Errorf("a voice must not be lowered like an ambience: %s", args)
	}
}
