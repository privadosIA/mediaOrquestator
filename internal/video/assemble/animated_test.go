package assemble

import (
	"strings"
	"testing"
)

func animatedScene(duration float64) Scene {
	return Scene{
		ID: "s1", ImagePath: "img.png", AudioPath: "a.wav", Duration: duration,
		AnimatedPath: "tmp/scene_s1.anim.mp4", AnimatedDuration: 4,
		Width: 1280, Height: 720,
	}
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("flag %s not in %v", flag, args)
	return ""
}

func TestAnimatedSlowdown(t *testing.T) {
	for _, tc := range []struct{ d, s, want float64 }{
		{5, 4, 1.25}, // a bit longer: stretch to fit
		{20, 4, 2},   // much longer: capped, rest is looped
		{3, 4, 1},    // shorter: cut, no speed change
		{4, 4, 1},    // equal
		{5, 0, 1},    // unknown source length: no change
	} {
		if got := AnimatedSlowdown(tc.d, tc.s); got != tc.want {
			t.Errorf("AnimatedSlowdown(%v, %v) = %v, want %v", tc.d, tc.s, got, tc.want)
		}
	}
}

func TestBuildClipArgs_Animated(t *testing.T) {
	cases := []struct {
		name     string
		duration float64
		wantSetp string
		wantT    string
	}{
		{"narration longer than the animation", 5, "setpts=1.250000*PTS", "5.000000"},
		{"narration much longer", 20, "setpts=2.000000*PTS", "20.000000"},
		{"narration shorter", 3, "setpts=1.000000*PTS", "3.000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := BuildClipArgs(animatedScene(tc.duration), 30, "out.mp4")
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-stream_loop -1 -i tmp/scene_s1.anim.mp4") {
				t.Errorf("animated input must be looped: %v", args)
			}
			if strings.Contains(joined, "-loop 1") || strings.Contains(joined, "stillimage") {
				t.Errorf("animated clip must not use the looped still: %v", args)
			}
			vf := flagValue(t, args, "-vf")
			for _, want := range []string{tc.wantSetp, "fps=30", "scale=1280:720:force_original_aspect_ratio=increase", "crop=1280:720"} {
				if !strings.Contains(vf, want) {
					t.Errorf("-vf %q missing %q", vf, want)
				}
			}
			if got := flagValue(t, args, "-t"); got != tc.wantT {
				t.Errorf("-t = %s, want %s", got, tc.wantT)
			}
		})
	}
}

func TestBuildClipArgs_AnimatedIgnoresMotionKeepsLook(t *testing.T) {
	s := animatedScene(5)
	s.MotionType = "zoom_in"
	s.Grain = 0.5
	s.WhiteBalance = "warm"
	vf := flagValue(t, BuildClipArgs(s, 30, "out.mp4"), "-vf")
	if strings.Contains(vf, "zoompan") {
		t.Errorf("motion must not apply to animated scenes: %s", vf)
	}
	if !strings.Contains(vf, "colortemperature=temperature=4600") || !strings.Contains(vf, "noise=alls=10") {
		t.Errorf("look must still apply: %s", vf)
	}
}

func TestBuildClipArgs_AnimatedKeepsEncodeParameters(t *testing.T) {
	still := BuildClipArgs(Scene{ID: "s", ImagePath: "i.png", AudioPath: "a.wav", Duration: 5}, 30, "out.mp4")
	anim := BuildClipArgs(animatedScene(5), 30, "out.mp4")
	for _, flag := range []string{"-c:v", "-pix_fmt", "-c:a", "-b:a", "-ar", "-ac"} {
		if flagValue(t, still, flag) != flagValue(t, anim, flag) {
			t.Errorf("%s differs between still and animated clips", flag)
		}
	}
}

func TestBuildClipArgs_AnimatedSilentScene(t *testing.T) {
	s := animatedScene(2.5)
	s.Silent = true
	joined := strings.Join(BuildClipArgs(s, 30, "out.mp4"), " ")
	if !strings.Contains(joined, "-f lavfi -i "+SilentAudioSource) {
		t.Errorf("silent animated scene must use the generated track: %s", joined)
	}
}
