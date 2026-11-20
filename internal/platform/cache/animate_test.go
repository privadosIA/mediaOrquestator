package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func baseAnimate() AnimateParams {
	return AnimateParams{Prompt: "move", Seed: 1, DiffusionModel: "d", T5XXL: "t", VAE: "v",
		Resolution: "832x480", Frames: 33, FPS: 16, Steps: 20, CfgScale: 6}
}

func TestAnimateHash_EveryFieldChangesIt(t *testing.T) {
	base := AnimateHash(baseAnimate())
	for name, mut := range map[string]func(p *AnimateParams){
		"prompt":     func(p *AnimateParams) { p.Prompt = "x" },
		"seed":       func(p *AnimateParams) { p.Seed = 2 },
		"diffusion":  func(p *AnimateParams) { p.DiffusionModel = "d2" },
		"t5xxl":      func(p *AnimateParams) { p.T5XXL = "t2" },
		"vae":        func(p *AnimateParams) { p.VAE = "v2" },
		"resolution": func(p *AnimateParams) { p.Resolution = "640x480" },
		"frames":     func(p *AnimateParams) { p.Frames = 65 },
		"fps":        func(p *AnimateParams) { p.FPS = 24 },
		"steps":      func(p *AnimateParams) { p.Steps = 30 },
		"cfg":        func(p *AnimateParams) { p.CfgScale = 5 },
	} {
		p := baseAnimate()
		mut(&p)
		if AnimateHash(p) == base {
			t.Errorf("changing %s must change the animate hash", name)
		}
	}
}

func TestClipHash_UnchangedWithoutAnimate(t *testing.T) {
	p := ClipParams{MotionType: "static", MotionIntensity: 0.5, FPS: 30, Seed: 3}
	h := sha256.New()
	fmt.Fprintf(h, "motion_type=static\x00motion_intensity=0.5\x00grain=0\x00white_balance=\x00fps=30\x00silence_seconds=0\x00seed=3\x00")
	if got, want := ClipHash(p), hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("clip hash without animate = %s, want legacy %s", got, want)
	}
	p.AnimateHash = "abc"
	if ClipHash(p) == hex.EncodeToString(h.Sum(nil)) {
		t.Error("animate hash must change the clip hash")
	}
}

func animatedManifest() *Manifest {
	return &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a", ImageHash: "i", ClipHash: "c", AnimateHash: "n"},
	}}
}

func TestDecideAnimated(t *testing.T) {
	m := animatedManifest()
	no := OnlyFilter{}

	d := DecideAnimated(m, no, "s1", "a", "i", "c", "n", false)
	for _, p := range []Phase{PhaseTTS, PhaseRender, PhaseAnimate, PhaseClip} {
		if !d.Phases[p] {
			t.Errorf("all hits: %s should be a hit", p)
		}
	}

	// A changed motion prompt re-animates (and re-clips) but keeps the render.
	d = DecideAnimated(m, no, "s1", "a", "i", "c2", "n2", false)
	if !d.Phases[PhaseRender] || d.Phases[PhaseAnimate] || d.Phases[PhaseClip] {
		t.Errorf("new animate hash: render hit, animate/clip miss; got %v", d.Phases)
	}

	// A changed image re-animates.
	d = DecideAnimated(m, no, "s1", "a", "i2", "c", "n", false)
	if d.Phases[PhaseRender] || d.Phases[PhaseAnimate] {
		t.Errorf("new image: render and animate miss; got %v", d.Phases)
	}

	// Toggling animate on (no stored animate hash) re-encodes the clip.
	plain := &Manifest{Scenes: map[string]SceneEntry{"s1": {AudioHash: "a", ImageHash: "i", ClipHash: "c"}}}
	d = DecideAnimated(plain, no, "s1", "a", "i", "c2", "n", false)
	if d.Phases[PhaseAnimate] || d.Phases[PhaseClip] {
		t.Errorf("first animation: animate and clip miss; got %v", d.Phases)
	}

	// --force misses everything; --only reports other scenes cached.
	d = DecideAnimated(m, no, "s1", "a", "i", "c", "n", true)
	if d.Phases[PhaseAnimate] || d.Phases[PhaseClip] {
		t.Errorf("force: miss; got %v", d.Phases)
	}
	d = DecideAnimated(m, OnlyFilter{SceneID: "other"}, "s1", "x", "x", "x", "x", false)
	if !d.Phases[PhaseAnimate] || !d.Phases[PhaseClip] {
		t.Errorf("excluded by --only: hit; got %v", d.Phases)
	}
}

func TestDecide_NonAnimatedHasNoAnimatePhase(t *testing.T) {
	d := Decide(animatedManifest(), "s1", "a", "i", "c", false)
	if _, ok := d.Phases[PhaseAnimate]; ok {
		t.Error("a non-animated decision must not carry the animate phase")
	}
}

func TestAnimatePaths(t *testing.T) {
	raw, mp4 := AnimatePaths("intro")
	if raw != "tmp/scene_intro.anim.avi" || mp4 != "tmp/scene_intro.anim.mp4" {
		t.Errorf("paths = %q, %q", raw, mp4)
	}
}

// A project without a high-noise expert must keep the animate key it had
// before the two-expert mode existed (captured from that version), so its
// clips stay cache hits.
func TestAnimateHash_SingleModelKeyIsUnchanged(t *testing.T) {
	p := AnimateParams{Prompt: "p", Seed: 5, DiffusionModel: "d", T5XXL: "t", VAE: "v",
		Resolution: "832x480", Frames: 33, FPS: 16, Steps: 20, CfgScale: 6}
	const want = "696148c74ec527b3d4d8479c82d08c1eee059810a59ba55dfb57ffa7a3913093"
	if got := AnimateHash(p); got != want {
		t.Errorf("single-model animate hash changed: %s, want %s", got, want)
	}
	// Leftover two-expert values without the expert itself must not matter.
	p.HighNoiseSteps, p.HighNoiseCfgScale, p.FlowShift = 8, 3.5, "3.0"
	if got := AnimateHash(p); got != want {
		t.Errorf("high-noise values without the expert must be ignored: %s", got)
	}
}

func TestAnimateHash_TwoExpertFieldsChangeIt(t *testing.T) {
	two := func() AnimateParams {
		p := baseAnimate()
		p.HighNoiseModel, p.HighNoiseSteps, p.HighNoiseCfgScale, p.FlowShift = "high.gguf", 8, 3.5, "3.0"
		return p
	}
	base := AnimateHash(two())
	if base == AnimateHash(baseAnimate()) {
		t.Fatal("turning the expert on must change the key")
	}
	for name, mut := range map[string]func(p *AnimateParams){
		"expert":     func(p *AnimateParams) { p.HighNoiseModel = "other.gguf" },
		"high steps": func(p *AnimateParams) { p.HighNoiseSteps = 6 },
		"high cfg":   func(p *AnimateParams) { p.HighNoiseCfgScale = 0 },
		"flow shift": func(p *AnimateParams) { p.FlowShift = "5.0" },
	} {
		p := two()
		mut(&p)
		if AnimateHash(p) == base {
			t.Errorf("changing %s must change the animate hash", name)
		}
	}
}

func animateDiffParams() AnimateParams {
	return AnimateParams{Prompt: "move", Seed: 1, Engine: "animatediff", Checkpoint: "sd15", MotionModule: "mm",
		Strength: 0.75, Resolution: "512x512", Frames: 16, FPS: 8, Steps: 25, CfgScale: 7}
}

func TestAnimateHash_AnimateDiffEveryFieldChangesItAndWanModelsDoNot(t *testing.T) {
	base := AnimateHash(animateDiffParams())
	for name, mut := range map[string]func(p *AnimateParams){
		"checkpoint": func(p *AnimateParams) { p.Checkpoint = "other" },
		"module":     func(p *AnimateParams) { p.MotionModule = "mm2" },
		"strength":   func(p *AnimateParams) { p.Strength = 0.6 },
		"frames":     func(p *AnimateParams) { p.Frames = 24 },
		"steps":      func(p *AnimateParams) { p.Steps = 30 },
		"cfg":        func(p *AnimateParams) { p.CfgScale = 5 },
		"fps":        func(p *AnimateParams) { p.FPS = 12 },
		"resolution": func(p *AnimateParams) { p.Resolution = "640x352" },
		"prompt":     func(p *AnimateParams) { p.Prompt = "x" },
		"seed":       func(p *AnimateParams) { p.Seed = 2 },
	} {
		p := animateDiffParams()
		mut(&p)
		if AnimateHash(p) == base {
			t.Errorf("changing %s must change the AnimateDiff hash", name)
		}
	}
	p := animateDiffParams()
	p.DiffusionModel, p.T5XXL, p.VAE = "d", "t", "v"
	if AnimateHash(p) != base {
		t.Error("Wan models are not inputs of an AnimateDiff clip")
	}
}

func TestAnimateHash_EngineSwitchChangesTheKey(t *testing.T) {
	wan := baseAnimate()
	ad := animateDiffParams()
	ad.Resolution, ad.Frames, ad.FPS, ad.Steps, ad.CfgScale = wan.Resolution, wan.Frames, wan.FPS, wan.Steps, wan.CfgScale
	if AnimateHash(wan) == AnimateHash(ad) {
		t.Error("Wan and AnimateDiff must not share a key")
	}
}

func TestAnimateHash_TheInputsCountAndAbsentOnesKeepTheKey(t *testing.T) {
	for _, engine := range []string{"minimax-h3", "ltx-2.3"} {
		base := baseAnimate()
		base.Engine = engine
		h := AnimateHash(base)
		withEnd := base
		withEnd.EndImage = "aa"
		other := base
		other.EndImage = "bb"
		refs := base
		refs.References = []string{"image:1", "video:2"}
		swapped := base
		swapped.References = []string{"video:2", "image:1"}
		fewer := base
		fewer.References = []string{"image:1"}
		seen := map[string]string{"none": h}
		for name, p := range map[string]AnimateParams{"end": withEnd, "other end": other, "refs": refs, "swapped": swapped, "fewer": fewer} {
			x := AnimateHash(p)
			for n, prev := range seen {
				if x == prev {
					t.Errorf("%s: %s and %s hash the same", engine, name, n)
				}
			}
			seen[name] = x
		}
		empty := base
		empty.References = []string{}
		if AnimateHash(empty) != h {
			t.Errorf("%s: an empty list must keep the key", engine)
		}
	}
}
