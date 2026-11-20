package generation

import (
	"testing"
)

func machine(vram, ram int) planMachine { return planMachine{VRAMMB: vram, RAMGB: ram, Speed: 1} }

func availableIDs(cs []videoChoice) map[string]bool {
	out := map[string]bool{}
	for _, c := range cs {
		if c.Available {
			out[c.ID] = true
		}
	}
	return out
}

func TestMachineKnowsTheCardsAndGuessesTheUnknownOnes(t *testing.T) {
	if m := MachineOf([]gpuStatus{{Name: "NVIDIA GeForce RTX 5060 Ti", TotalVRAMMB: 16384}}, 32<<30); m.Speed != 1 || m.VRAMMB != 16384 || m.RAMGB != 32 {
		t.Errorf("5060 Ti: %+v", m)
	}
	if m := MachineOf([]gpuStatus{{Name: "Mystery GPU", TotalVRAMMB: 8000}}, 0); m.Speed != 1.5 || m.RAMGB != 0 {
		t.Errorf("unknown card: %+v", m)
	}
	if m := MachineOf(nil, 16<<30); m.VRAMMB != 0 || m.Speed != 20 {
		t.Errorf("no card: %+v", m)
	}
}

func TestMachineUsesTheMeasuredSpeedAndTheBiggestCardByTotalMemory(t *testing.T) {
	m := MachineOf([]gpuStatus{
		{Name: "RTX 4070", TotalVRAMMB: 12288, FreeVRAMMB: 2000, SpeedFactor: 0.9, SpeedSamples: 5},
		{Name: "RTX 3060", TotalVRAMMB: 8192, FreeVRAMMB: 8000},
	}, 0)
	if m.VRAMMB != 12288 || m.Speed != 0.9 {
		t.Errorf("machine = %+v, want 12288 MB at 0.9", m)
	}
}

func TestVideoChoicesHaveStableUniqueIDs(t *testing.T) {
	cs := videoChoices(baseCatalog(), machine(16384, 32))
	want := []string{"h3-full", "wan-high", "ltx", "h3-draft", "wan-medium", "wan-low", "ad-high", "ad-medium", "ad-low", "h3-ref", "h3-ref-draft"}
	if len(cs) != len(want) {
		t.Fatalf("%d choices, want %d", len(cs), len(want))
	}
	seen := map[string]bool{}
	for i, c := range cs {
		if c.ID != want[i] || seen[c.ID] {
			t.Errorf("choice %d = %q, want %q (unique)", i, c.ID, want[i])
		}
		seen[c.ID] = true
		if !c.Available && c.Reason == "" {
			t.Errorf("%s is unavailable with no reason", c.ID)
		}
	}
}

func TestVideoChoicesOn16GB(t *testing.T) {
	cs := videoChoices(baseCatalog(), machine(16384, 32))
	got := availableIDs(cs)
	for _, id := range []string{"h3-full", "h3-draft", "wan-high", "wan-medium", "wan-low"} {
		if !got[id] {
			t.Errorf("%s should be available on 16 GB", id)
		}
	}
	for _, id := range []string{"ltx", "ad-high", "ad-medium", "ad-low"} {
		if got[id] {
			t.Errorf("%s is not installed and cannot be available", id)
		}
	}
	if c, _ := findChoice(cs, "ltx"); c.Reason != "LTX 2.3 is not installed" {
		t.Errorf("ltx reason = %q", c.Reason)
	}
}

func TestH3IsLeftOutWhenTheCardOrTheRAMIsTooSmall(t *testing.T) {
	for name, m := range map[string]planMachine{"12 GB": machine(12288, 32), "little RAM": machine(16384, 16)} {
		cs := videoChoices(baseCatalog(), m)
		c, _ := findChoice(cs, "h3-full")
		if c.Available || c.Reason == "" {
			t.Errorf("%s: h3-full = %+v, want unavailable with a reason", name, c)
		}
		if d, _ := findChoice(cs, "h3-draft"); d.Available {
			t.Errorf("%s: h3-draft should be unavailable too", name)
		}
	}
	if c, _ := findChoice(videoChoices(baseCatalog(), machine(12288, 32)), "wan-high"); !c.Available {
		t.Errorf("a 12 GB card takes Wan A14B: %+v", c)
	}
}

func TestLTXFitsA12GBCardAtItsPreset(t *testing.T) {
	cat := append(baseCatalog(), tLTX())
	c, _ := findChoice(videoChoices(cat, machine(12288, 32)), "ltx")
	if !c.Available {
		t.Fatalf("ltx = %+v", c)
	}
	c, _ = findChoice(videoChoices(cat, machine(8192, 32)), "ltx")
	if c.Available || c.Reason == "" {
		t.Errorf("ltx on 8 GB = %+v, want a reason", c)
	}
}

func TestTheFirstFrameBundleIsPreferredAndRunsSmallerOnA12GBCard(t *testing.T) {
	cat := append(baseCatalog(), tH3FF())
	c, _ := findChoice(videoChoices(cat, machine(16384, 32)), "h3-full")
	if !c.Available || !c.FirstFrame || c.LowRes != "" {
		t.Errorf("16 GB: %+v", c)
	}
	c, _ = findChoice(videoChoices(cat, machine(12288, 32)), "h3-draft")
	if !c.Available || !c.FirstFrame || c.LowRes != "768x416" {
		t.Errorf("12 GB: %+v, want first frame at 768x416", c)
	}
	c, _ = findChoice(videoChoices(cat, machine(8192, 32)), "h3-full")
	if c.Available {
		t.Errorf("8 GB: %+v, want left out", c)
	}
}

func TestAnimateDiffNeedsItsBaseCheckpoint(t *testing.T) {
	cat := append(baseCatalog(), tAD())
	if c, _ := findChoice(videoChoices(cat, machine(8192, 32)), "ad-medium"); !c.Available {
		t.Errorf("ad-medium = %+v", c)
	}
	noBase := []catalogEntry{tAD(), tInto()}
	if c, _ := findChoice(videoChoices(noBase, machine(8192, 32)), "ad-low"); c.Available || c.Reason == "" {
		t.Errorf("without SD 1.5 = %+v", c)
	}
}

func TestOptionsDefaultIsTheFirstAvailable(t *testing.T) {
	opts := buildVideoOptions(baseCatalog(), machine(16384, 32), "")
	n := 0
	for _, o := range opts {
		if o.Default {
			n++
			if o.ID != "h3-full" {
				t.Errorf("default = %s, want h3-full", o.ID)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d defaults, want exactly one", n)
	}
	none := buildVideoOptions(nil, machine(16384, 32), "")
	for _, o := range none {
		if o.Default || o.Available {
			t.Errorf("empty catalog: %+v", o)
		}
	}
}

func TestOptionsDescribeWhatAClientNeeds(t *testing.T) {
	opts := buildVideoOptions(append(baseCatalog(), tLTX()), machine(16384, 32), "")
	for _, o := range opts {
		if o.Name == "" || o.Description == "" || o.Engine == "" || o.Duration.Min <= 0 || o.Duration.Max <= o.Duration.Min {
			t.Errorf("%s is incomplete: %+v", o.ID, o)
		}
		if o.Available && (o.ClipSeconds <= 0 || o.Estimate <= 0) {
			t.Errorf("%s: clip_seconds %v, estimate %d", o.ID, o.ClipSeconds, o.Estimate)
		}
		wantAudio := o.Engine == "minimax-h3" || o.Engine == "ltx-2.3"
		if o.Audio != wantAudio {
			t.Errorf("%s audio = %v", o.ID, o.Audio)
		}
	}
	h3, ltx := opts[0], opts[2]
	if h3.Estimate <= ltx.Estimate {
		t.Errorf("H3 (%d s) should take longer than LTX (%d s)", h3.Estimate, ltx.Estimate)
	}
}

func TestOptionsNeedAnImageModelForTheFirstPicture(t *testing.T) {
	cat := []catalogEntry{tWan("wan5q4", "low", 85, 20, 6144, false)}
	o := buildVideoOptions(cat, machine(16384, 32), "")
	for _, v := range o {
		if v.Available {
			t.Errorf("%s is available with no image model", v.ID)
		}
	}
	if o[5].ID != "wan-low" || o[5].Reason == "" {
		t.Errorf("wan-low = %+v", o[5])
	}
}

func TestImageModelsListTheEditorsFirstAndTheDefaultIsTheFirstAvailable(t *testing.T) {
	cat := []catalogEntry{tInto(), tQwen(), tKontext(), tSD15()}
	ms := buildImageModels(cat, machine(16384, 32))
	ids := []string{}
	for _, m := range ms {
		ids = append(ids, m.ID)
	}
	want := []string{"kontext", "qwen", "into", "sd-1-5"}
	for i := range want {
		if i >= len(ids) || ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
	if !ms[0].Default || ms[1].Default {
		t.Errorf("defaults: %+v", ms)
	}
	if ms[0].Kind != "instruction" || ms[0].Strength || ms[0].NegativePrompt || ms[0].MaxVariants != 1 {
		t.Errorf("kontext = %+v", ms[0])
	}
	if ms[2].Kind != "img2img" || !ms[2].Strength || !ms[2].NegativePrompt || ms[2].MaxVariants != 8 {
		t.Errorf("into = %+v", ms[2])
	}
}

func TestImageModelThatCannotRun(t *testing.T) {
	q := tQwen()
	q.Installed = false
	ms := buildImageModels([]catalogEntry{q, tKontext()}, machine(16384, 32))
	if ms[1].ID != "qwen" || ms[1].Available || ms[1].Reason == "" {
		t.Errorf("qwen = %+v", ms[1])
	}
	if !ms[0].Default {
		t.Errorf("default should be kontext: %+v", ms[0])
	}
	small := buildImageModels([]catalogEntry{tQwen()}, machine(8192, 32))
	if small[0].Available || small[0].Reason == "" {
		t.Errorf("qwen on 8 GB = %+v", small[0])
	}
}

func TestShapeResKeepsThePixelsAndTheMultiple(t *testing.T) {
	for _, tc := range []struct {
		base         string
		aw, ah       int
		scale        float64
		multiple     int
		wantW, wantH int
	}{
		{"864x480", 16, 9, 1, 32, 864, 480},
		{"864x480", 9, 16, 1, 32, 480, 864},
		{"832x480", 1, 1, 1, 16, 624, 624},
		{"864x480", 16, 9, 0.5, 32, 416, 224 + 32}, // never under 256
	} {
		var w, h int
		got := shapeRes(tc.base, tc.aw, tc.ah, tc.scale, tc.multiple)
		if _, err := fmtSscan(got, &w, &h); err != nil || w%tc.multiple != 0 || h%tc.multiple != 0 || w < 256 || h < 256 {
			t.Errorf("shapeRes(%s) = %s", tc.base, got)
		}
		if tc.scale == 1 && (w != tc.wantW || h != tc.wantH) {
			t.Errorf("shapeRes(%s, %d:%d) = %s, want %dx%d", tc.base, tc.aw, tc.ah, got, tc.wantW, tc.wantH)
		}
	}
}

func TestFitResKeepsTheShapeInMultiplesOf64(t *testing.T) {
	if got := fitRes("1280x704", 1280*704); got != "1280x704" {
		t.Errorf("unchanged = %s", got)
	}
	got := fitRes("1408x768", 1280*704)
	w, h, _ := parseRes(got)
	if w*h > 1280*704 || w%64 != 0 || h%64 != 0 {
		t.Errorf("fit = %s", got)
	}
}

func fmtSscan(res string, w, h *int) (int, error) {
	a, b, ok := parseRes(res)
	if !ok {
		return 0, errBadRes
	}
	*w, *h = a, b
	return 2, nil
}

var errBadRes = errString("bad resolution")

type errString string

func (e errString) Error() string { return string(e) }
