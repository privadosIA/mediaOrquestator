package cache

import "testing"

func baseClipParams() ClipParams {
	return ClipParams{
		MotionType:      "static",
		MotionIntensity: 0.5,
		Grain:           0.0,
		WhiteBalance:    "neutral",
		FPS:             30,
		SilenceSeconds:  0,
		Seed:            42,
	}
}

func TestClipHash_DefaultsAreStable(t *testing.T) {
	a := ClipHash(baseClipParams())
	b := ClipHash(baseClipParams())
	if a != b {
		t.Fatal("ClipHash must be deterministic for identical inputs")
	}
}

func TestClipHash_EachFieldChangesDigest(t *testing.T) {
	base := ClipHash(baseClipParams())
	cases := []struct {
		name   string
		mutate func(*ClipParams)
	}{
		{"motion_type", func(p *ClipParams) { p.MotionType = "zoom_in" }},
		{"motion_intensity", func(p *ClipParams) { p.MotionIntensity = 0.9 }},
		{"grain", func(p *ClipParams) { p.Grain = 0.3 }},
		{"white_balance", func(p *ClipParams) { p.WhiteBalance = "warm" }},
		{"fps", func(p *ClipParams) { p.FPS = 24 }},
		{"silence_seconds", func(p *ClipParams) { p.SilenceSeconds = 2.0 }},
		{"seed", func(p *ClipParams) { p.Seed = 43 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := baseClipParams()
			tc.mutate(&p)
			if ClipHash(p) == base {
				t.Errorf("changing %s did not change the digest", tc.name)
			}
		})
	}
}

func TestDecide_ClipParamsChangeInvalidatesOnlyClip(t *testing.T) {
	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a1", ImageHash: "i1", ClipHash: "c1"},
	}}
	d := Decide(m, "s1", "a1", "i1", "c2", false)
	if !d.Phases[PhaseTTS] || !d.Phases[PhaseRender] {
		t.Error("tts and render should hit when audio and image hashes match")
	}
	if d.Phases[PhaseClip] {
		t.Error("clip should miss when the clip-parameters hash changed")
	}
}

func TestDecide_LegacyEntryWithoutClipHashIsClipMiss(t *testing.T) {
	m := &Manifest{Scenes: map[string]SceneEntry{
		"s1": {AudioHash: "a1", ImageHash: "i1"}, // pre-change manifest: no ClipHash
	}}
	d := Decide(m, "s1", "a1", "i1", "c1", false)
	if !d.Phases[PhaseTTS] || !d.Phases[PhaseRender] {
		t.Error("tts and render are unaffected by the clip-hash migration")
	}
	if d.Phases[PhaseClip] {
		t.Error("a legacy entry without a stored clip hash must be a clip miss")
	}

	// After the re-run stores the hash, the next evaluation hits.
	m.UpdateEntry("s1", SceneEntry{AudioHash: "a1", ImageHash: "i1", ClipHash: "c1"})
	d = Decide(m, "s1", "a1", "i1", "c1", false)
	if !d.Phases[PhaseClip] {
		t.Error("clip should hit once the hash is stored and matches")
	}
}
