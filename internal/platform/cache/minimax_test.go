package cache

import "testing"

func TestAnimateHash_MiniMaxUsesItsOwnFiles(t *testing.T) {
	base := AnimateParams{Prompt: "p", Seed: 1, Engine: "minimax-h3", DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l",
		Resolution: "864x480", Frames: 56, FPS: 24, Steps: 20, CfgScale: 1}
	h := AnimateHash(base)
	for name, mut := range map[string]func(*AnimateParams){
		"dit":    func(p *AnimateParams) { p.DiffusionModel = "dit2" },
		"vae":    func(p *AnimateParams) { p.VAE = "v2" },
		"audio":  func(p *AnimateParams) { p.AudioVAE = "a2" },
		"llm":    func(p *AnimateParams) { p.LLM = "l2" },
		"frames": func(p *AnimateParams) { p.Frames = 39 },
	} {
		p := base
		mut(&p)
		if AnimateHash(p) == h {
			t.Errorf("changing %s must change the key", name)
		}
	}
	wan := base
	wan.Engine = ""
	if AnimateHash(wan) == h {
		t.Error("the engine must be part of the key")
	}
}

func TestAnimateHash_MiniMaxLoRAChangesTheHashOnlyWhenSet(t *testing.T) {
	base := AnimateParams{Prompt: "p", Seed: 1, Engine: "minimax-h3", DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l"}
	with := base
	with.LoRA = "models/video/turbo.safetensors"
	if AnimateHash(base) == AnimateHash(with) {
		t.Error("a LoRA must change the hash")
	}
}

func TestAnimateHash_MiniMaxFirstFrameChangesTheHashOnlyWhenSet(t *testing.T) {
	base := AnimateParams{Prompt: "p", Seed: 1, Engine: "minimax-h3", DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l"}
	with := base
	with.FirstFrame = true
	if AnimateHash(base) == AnimateHash(with) {
		t.Error("the first-frame mode must change the hash")
	}
}

func TestAnimateHash_LTXUsesItsOwnFiles(t *testing.T) {
	base := AnimateParams{Prompt: "p", Seed: 1, Engine: "ltx-2.3", DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l", Connectors: "c",
		Resolution: "768x416", Frames: 49, FPS: 24, Steps: 8, CfgScale: 1}
	h := AnimateHash(base)
	for name, mut := range map[string]func(*AnimateParams){
		"dit":        func(p *AnimateParams) { p.DiffusionModel = "dit2" },
		"vae":        func(p *AnimateParams) { p.VAE = "v2" },
		"audio":      func(p *AnimateParams) { p.AudioVAE = "a2" },
		"llm":        func(p *AnimateParams) { p.LLM = "l2" },
		"connectors": func(p *AnimateParams) { p.Connectors = "c2" },
		"frames":     func(p *AnimateParams) { p.Frames = 33 },
	} {
		p := base
		mut(&p)
		if AnimateHash(p) == h {
			t.Errorf("changing %s must change the key", name)
		}
	}
	h3 := base
	h3.Engine = "minimax-h3"
	if AnimateHash(h3) == h {
		t.Error("the engine must be part of the key")
	}
}

func TestAnimateHash_LTXDoesNotChangeOtherEngines(t *testing.T) {
	h3 := AnimateParams{Prompt: "p", Seed: 1, Engine: "minimax-h3", DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l"}
	withConn := h3
	withConn.Connectors = "c" // never hashed outside ltx-2.3
	if AnimateHash(h3) != AnimateHash(withConn) {
		t.Error("connectors must not affect the key of other engines")
	}
}

func TestAnimateHash_ModelAudioChangesTheHashOnlyWhenSet(t *testing.T) {
	for _, engine := range []string{"minimax-h3", "ltx-2.3"} {
		base := AnimateParams{Prompt: "p", Seed: 1, Engine: engine, DiffusionModel: "dit", VAE: "v", AudioVAE: "a", LLM: "l", Connectors: "c",
			Resolution: "768x416", Frames: 49, FPS: 24, Steps: 8, CfgScale: 1}
		with := base
		with.ModelAudio = true
		if AnimateHash(base) == AnimateHash(with) {
			t.Errorf("%s: model_audio must change the hash", engine)
		}
		off := base
		off.ModelAudio = false
		if AnimateHash(off) != AnimateHash(base) {
			t.Errorf("%s: an unset model_audio must keep the old hash", engine)
		}
	}
}
