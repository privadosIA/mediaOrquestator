package musicmotion

import (
	"strings"
	"testing"
)

func repairDesign() Design {
	return Design{Dark: "#0a0a12", Light: "#f4f4f8", Accent: "#00ffcc", DefaultMode: ModeDark}
}

func TestARepairedColourPassesTheColourCheck(t *testing.T) {
	frag := `<style>.a{background:linear-gradient(to top, #000, transparent);color:#fff}.b{background:rgba(0,0,0,0.8);border:1px solid rgba(0,255,204,0.1)}</style><div class="a"></div>`
	if len(sceneStyleProblems(frag)) == 0 {
		t.Fatal("the fixture must fail the check")
	}
	out, reps := RepairScene(frag, repairDesign())
	if p := sceneStyleProblems(out); len(p) > 0 {
		t.Fatalf("the repaired fragment still fails: %v\n%s", p, out)
	}
	if len(reps) != 1 || reps[0].Kind != RepairColour || reps[0].Count != 4 {
		t.Fatalf("repairs = %+v", reps)
	}
	if !strings.Contains(out, "color-mix(in srgb, var(--bg) 80%, transparent)") {
		t.Errorf("an rgba of black keeps its opacity as a mix with the background: %s", out)
	}
	if !strings.Contains(out, "color-mix(in srgb, var(--accent) 10%, transparent)") {
		t.Errorf("a translucent accent keeps its opacity: %s", out)
	}
}

func TestALightDesignMapsBlackToTheForeground(t *testing.T) {
	d := repairDesign()
	d.DefaultMode = ModeLight
	out, _ := RepairScene(`<style>.a{color:#000}</style>`, d)
	if !strings.Contains(out, "var(--fg)") {
		t.Errorf("in a light design black is the text colour: %s", out)
	}
}

func TestNeutralShadowsVariablesAndUrlsAreLeftAlone(t *testing.T) {
	frag := `<style>.a{text-shadow:0 0 8px rgba(0,0,0,0.5);color:var(--fg);fill:url(#grad);background:var(--bg)}</style>`
	out, reps := RepairScene(frag, repairDesign())
	if out != frag || len(reps) != 0 {
		t.Errorf("nothing to repair, got %q %+v", out, reps)
	}
}

func TestAFontOfItsOwnBecomesTheDisplayFont(t *testing.T) {
	frag := `<style>.a{font-family: varvare, sans-serif}.b{font-family:var(--body)}</style><svg><text font-family="Arial"></text></svg>`
	out, reps := RepairScene(frag, repairDesign())
	if len(reps) != 1 || reps[0].Kind != RepairFont || reps[0].Count != 2 {
		t.Fatalf("repairs = %+v", reps)
	}
	if p := sceneStyleProblems(out); len(p) > 0 {
		t.Fatalf("still fails: %v\n%s", p, out)
	}
	if !strings.Contains(out, "font-family:var(--body)") {
		t.Errorf("a font that comes from the design is untouched: %s", out)
	}
}

func TestMathRandomBecomesASeededGenerator(t *testing.T) {
	frag := `<div></div><script>var x = Math.random(); window.__partSeek = function(t){ return Math.random() * t }</script>`
	out, reps := RepairScene(frag, repairDesign())
	if strings.Contains(out, "Math.random") {
		t.Fatalf("Math.random is still there: %s", out)
	}
	if len(reps) != 1 || reps[0].Kind != RepairClock || reps[0].Count != 2 {
		t.Fatalf("repairs = %+v", reps)
	}
	if !strings.Contains(out, "function __rnd()") || strings.Count(out, "__rnd()") < 3 {
		t.Errorf("the generator is defined once and used where Math.random was: %s", out)
	}
}

func TestAColourItCannotReadIsLeftForTheModel(t *testing.T) {
	frag := `<style>.a{color:rebeccapurple}</style>`
	out, reps := RepairScene(frag, repairDesign())
	if out != frag || len(reps) != 0 {
		t.Errorf("a named colour the program does not read is left as it is: %q %+v", out, reps)
	}
}

func TestParseColour(t *testing.T) {
	for in, want := range map[string]rgb{"#fff": {255, 255, 255}, "#000000": {0, 0, 0}, "rgb(10, 20, 30)": {10, 20, 30}, "rgba(0 255 204 / 0.1)": {0, 255, 204}, "black": {0, 0, 0}, "hsl(0, 100%, 50%)": {255, 0, 0}} {
		if c := parseColour(in); !c.ok || c.rgb != want {
			t.Errorf("%s = %+v, want %+v", in, c, want)
		}
	}
	if parseColour("rebeccapurple").ok || parseColour("#12").ok {
		t.Error("what is not a colour the program reads is not ok")
	}
	if a := parseColour("rgba(0,0,0,0.4)").a; a < 0.39 || a > 0.41 {
		t.Errorf("alpha = %v", a)
	}
}

func TestAVarWrittenWithLettersMoreAndAStyleClosedWithATypoAreRepaired(t *testing.T) {
	frag := `<style>.a{color:varvare(--fg);font-family:varine(--display)}</sc><div class="a">x</div>`
	out, reps := RepairScene(frag, repairDesign())
	if !strings.Contains(out, "color:var(--fg)") || !strings.Contains(out, "font-family:var(--display)") || !strings.Contains(out, "</style>") || strings.Contains(out, "</sc>") {
		t.Fatalf("%s", out)
	}
	if len(reps) != 1 || reps[0].Kind != RepairTypo || reps[0].Count != 3 {
		t.Fatalf("%+v", reps)
	}
	ok := `<style>.a{color:var(--fg)}</style><span>x</span>`
	if out, reps := RepairScene(ok, repairDesign()); out != ok || len(reps) != 0 {
		t.Errorf("a clean fragment is not touched: %q %+v", out, reps)
	}
}
