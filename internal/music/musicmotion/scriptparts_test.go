package musicmotion

import (
	"fmt"
	"testing"
)

// countingParts counts how the catalog asks a store for parts.
type countingParts struct {
	PartStore
	gets, getManys int
}

func (c *countingParts) Get(name string) (Part, bool) {
	c.gets++
	return c.PartStore.Get(name)
}

func (c *countingParts) GetMany(names []string) map[string]Part {
	c.getManys++
	return c.PartStore.GetMany(names)
}

func scriptOfParts(n int) Script {
	var s Script
	for i := 0; i < n; i++ {
		p := Piece{ID: fmt.Sprintf("p%d", i+1), Start: float64(i) * 4, End: float64(i+1) * 4, Part: fmt.Sprintf("user-part-%02d", i%7)}
		if i%3 == 0 {
			p.With = []PartRef{{Part: "user-part-03"}, {Part: "blank"}}
		}
		s.Pieces = append(s.Pieces, p)
	}
	s.Layers = []Layer{{Items: []LayerItem{{Part: "user-part-05"}, {Part: "user-part-06"}}}}
	return s
}

func TestAScriptNamesItsPartsOnceEachInOrder(t *testing.T) {
	names := scriptOfParts(10).PartNames()
	if len(names) == 0 || names[0] != "user-part-00" {
		t.Fatalf("%v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("%s twice in %v", n, names)
		}
		seen[n] = true
	}
	for _, want := range []string{"user-part-03", "blank", "user-part-05", "user-part-06"} {
		if !seen[want] {
			t.Errorf("%s (an extra or an item of a layer) is a part the script draws: %v", want, names)
		}
	}
}

func TestTheEstimateAndTheFallbackPiecesReadTheScriptsPartsWithOneQueryEach(t *testing.T) {
	st := memStore(t)
	db := NewDBParts(st)
	for i := 0; i < 7; i++ {
		if err := db.Create(manifestOf(fmt.Sprintf("user-part-%02d", i)), `<p>x</p>`, nil); err != nil {
			t.Fatal(err)
		}
	}
	// a catalog ten times larger must not change how many reads a script costs
	for i := 0; i < 70; i++ {
		if err := db.Create(manifestOf(fmt.Sprintf("other-part-%02d", i)), `<p>x</p>`, nil); err != nil {
			t.Fatal(err)
		}
	}
	counting := &countingParts{PartStore: db}
	cat := Catalog{Builtin: BuiltinParts(), Parts: counting}
	script := scriptOfParts(34)
	est := EstimateRender(script, cat, 30, 1280, 720, nil)
	if est.Frames == 0 || counting.gets != 0 || counting.getManys != 1 {
		t.Fatalf("the estimate: %d single reads and %d reads of many (want 0 and 1)", counting.gets, counting.getManys)
	}
	counting.gets, counting.getManys = 0, 0
	fb := FallbackPieces(script, cat)
	if len(fb) != 34 || counting.gets != 0 || counting.getManys != 1 {
		t.Fatalf("the fallback pieces: %d %d %d", len(fb), counting.gets, counting.getManys)
	}
	// the same answers as with a map that was read before
	parts := cat.GetMany(script.PartNames())
	if got := EstimateRenderWith(script, parts, 30, 1280, 720, nil); got != est {
		t.Fatalf("%+v vs %+v", got, est)
	}
}
