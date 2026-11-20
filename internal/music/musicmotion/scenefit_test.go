package musicmotion

import (
	"context"
	"strings"
	"testing"
)

func TestLayoutKindsReadTheProblemsOfTheChecks(t *testing.T) {
	got := LayoutKinds([]string{
		`the largest text of the scene is 5.1 vh (as drawn)`,
		`the text "We" is cut by the left edge of the frame at 3.5s`,
		`the texts "a" and "b" overlap at 3.5s: separate them`,
		`unreadable text`,
	})
	if strings.Join(got, ",") != "size,cut,overlap" {
		t.Errorf("%v", got)
	}
	if len(LayoutKinds([]string{"the picture covers 4 %"})) != 0 {
		t.Error("a problem the repair does not know asks for nothing")
	}
}

func TestRepairLayoutLeavesWhatItCannotWrapAndDoesNotRepeat(t *testing.T) {
	if out, reps := RepairLayout("<div>no script</div>", nil, []string{RepairSize}); out != "<div>no script</div>" || len(reps) != 0 {
		t.Errorf("%q %+v", out, reps)
	}
	once, reps := RepairLayout(probeScene("6vh", ""), []string{"x"}, []string{RepairSize})
	if len(reps) != 1 || !strings.Contains(once, fitMark) {
		t.Fatalf("%+v", reps)
	}
	if again, reps := RepairLayout(once, []string{"x"}, []string{RepairSize}); again != once || len(reps) != 0 {
		t.Error("a repaired fragment is not repaired twice")
	}
}

func fitProbe(t *testing.T, fragment string, kinds []string) (before, after string) {
	t.Helper()
	browser := browserOrSkip(t)
	in, err := BuildSceneInput(pieceSpec(t, Catalog{Builtin: BuiltinParts()}), SceneRef{Piece: "x", ScriptPiece: "p1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	check := func(f string) string {
		p, probs := ParsePart([]byte(probeManifest), f)
		if len(probs) > 0 {
			t.Fatal(probs)
		}
		got, err := CheckPart(context.Background(), browser, p, CheckOptions{Scene: &in.Scene, SceneRules: true})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(got, ";")
	}
	before = check(fragment)
	fixed, reps := RepairLayout(fragment, SceneKeywords(in.Scene), kinds)
	if len(reps) == 0 {
		t.Fatal("nothing was repaired")
	}
	return before, check(fixed)
}

func TestAWordUnderTheFloorIsRaisedInARealBrowser(t *testing.T) {
	before, after := fitProbe(t, probeScene("6vh", ""), []string{RepairSize})
	if !strings.Contains(before, "the word that matters is at least 12 vh") {
		t.Fatalf("the fixture must fail: %q", before)
	}
	if after != "" {
		t.Errorf("still refused: %s", after)
	}
}

func TestATextCutByTheEdgeAndTwoThatOverlapAreMovedInARealBrowser(t *testing.T) {
	cut := probeScene("14vh", "") + `<div id="c" style="position:absolute;left:-3vw;top:75vh;font-size:10vh;color:var(--fg)">the</div>`
	before, after := fitProbe(t, cut, []string{RepairCut})
	if !strings.Contains(before, "is cut by the left edge") {
		t.Fatalf("the fixture must fail: %q", before)
	}
	if strings.Contains(after, "is cut by") {
		t.Errorf("still cut: %s", after)
	}
	over := probeScene("14vh", "") + `<div style="position:absolute;left:30vw;top:30vh;font-size:14vh;color:var(--fg)">a</div><div style="position:absolute;left:31vw;top:31vh;font-size:10vh;color:var(--fg)">a</div>`
	before, after = fitProbe(t, over, []string{RepairOverlap})
	if !strings.Contains(before, "overlap at") {
		t.Fatalf("the fixture must fail: %q", before)
	}
	if strings.Contains(after, "overlap at") {
		t.Errorf("still overlapping: %s", after)
	}
}

func TestAPartThatDoesNotMoveIsToldToUsePartSeek(t *testing.T) {
	got := partMotionMessage([]string{"the page does not change over time: animate it with CSS animations, element.animate or window.__seek(t)", "other"})
	if !strings.Contains(got[0], "window.__partSeek") || strings.Contains(got[0], "CSS animations, element.animate") || got[1] != "other" {
		t.Fatalf("%q", got)
	}
}
