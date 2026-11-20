package musicmotion

import (
	"context"
	"testing"
)

func TestAPartWithATextParameterOrAHyperFramesOriginDrawsFixedCopy(t *testing.T) {
	no, yes := false, true
	text := []Param{{Name: "label", Type: "string", Default: "ALLOWED"}}
	colour := []Param{{Name: "accent", Type: "string", Default: "#00ffcc"}}
	empty := []Param{{Name: "label", Type: "string", Default: ""}}
	hf := &PartOrigin{Source: "hyperframes"}
	cases := []struct {
		name string
		m    Manifest
		o    *PartOrigin
		want bool
	}{
		{"a text default", Manifest{Params: text}, nil, true},
		{"a colour default is not text", Manifest{Params: colour}, nil, false},
		{"an empty text default", Manifest{Params: empty}, nil, false},
		{"no parameters", Manifest{}, nil, false},
		{"a HyperFrames block", Manifest{}, hf, true},
		{"the manifest says it has none", Manifest{Params: text, FixedCopy: &no}, nil, false},
		{"the manifest says it has", Manifest{FixedCopy: &yes}, nil, true},
		{"the manifest wins over the origin", Manifest{FixedCopy: &no}, hf, false},
	}
	for _, c := range cases {
		if got := fixedCopy(c.m, c.o); got != c.want {
			t.Errorf("%s: fixedCopy = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheFixedRuleNeverChoosesAPartWithFixedCopyAndNeverRepeatsOneInARow(t *testing.T) {
	for _, level := range Levels {
		input := in(level)
		fixed := map[string]bool{}
		for _, p := range input.Cat.List() {
			if p.HasFixedCopy {
				fixed[p.Name] = true
			}
		}
		if !fixed["stamp"] {
			t.Fatal("the built-in stamp (text APPROVED) must be marked as fixed copy")
		}
		p, err := ProposeScript(context.Background(), nil, input)
		if err != nil {
			t.Fatal(level, err)
		}
		for i, pc := range p.Script.Pieces {
			if fixed[pc.Part] {
				t.Errorf("%s: %s has fixed copy and was chosen by the fixed rule (level %s)", pc.ID, pc.Part, level)
			}
			if i > 0 && pc.Part == p.Script.Pieces[i-1].Part && !(IsChorus(pc.Section) && IsChorus(p.Script.Pieces[i-1].Section)) {
				t.Errorf("%s repeats %s of the piece before (level %s)", pc.ID, pc.Part, level)
			}
		}
	}
}

func TestNextPartSkipsThePartOfThePieceBefore(t *testing.T) {
	cands := []Part{{Manifest: Manifest{Name: "a"}}, {Manifest: Manifest{Name: "b"}}, {Manifest: Manifest{Name: "c"}}}
	if got := nextPart(cands, 0, ""); got != "a" {
		t.Errorf("no piece before: %s", got)
	}
	if got := nextPart(cands, 0, "a"); got != "b" {
		t.Errorf("the part before is skipped: %s", got)
	}
	if got := nextPart(cands[:1], 3, "a"); got != "a" {
		t.Errorf("a single candidate is used even when it repeats: %s", got)
	}
}

func TestAFallbackProposalNamesItsPiecesAndASceneTakesOneOut(t *testing.T) {
	input := in(LevelMedium)
	p, err := ProposeScript(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.FallbackPieces) != len(p.Script.Pieces) {
		t.Fatalf("every piece of a fallback script is a placeholder: %v of %d", p.FallbackPieces, len(p.Script.Pieces))
	}
	if got := FallbackPieces(p.Script, input.Cat); len(got) != len(p.Script.Pieces) {
		t.Fatalf("FallbackPieces = %v", got)
	}
}

func TestTheFixedRuleLeavesOutHyperFramesBlocks(t *testing.T) {
	dir := t.TempDir()
	m := []byte(`{"name":"hf-bg","description":"A background","version":1,"levels":["simple","medium","advanced"],"params":[],"fixed_copy":false}`)
	if err := WriteUserPart(dir, m, `<div></div>`, &PartOrigin{Source: "hyperframes"}); err != nil {
		t.Fatal(err)
	}
	cat := NewCatalog(nil)
	cat.UserDir = dir
	if p, ok := cat.Get("hf-bg"); !ok || p.HasFixedCopy {
		t.Fatalf("the block must be in the catalog without fixed copy: %+v %v", p, ok)
	}
	for _, c := range partsFor(cat, LevelSimple) {
		if c.Name == "hf-bg" {
			t.Fatal("a HyperFrames block is not chosen by the fixed rule")
		}
	}
}
