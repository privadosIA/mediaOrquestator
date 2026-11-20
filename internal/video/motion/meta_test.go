package motion

import (
	"strings"
	"testing"
)

func TestMetaOriginFollowsTheStoredPage(t *testing.T) {
	m := Meta{Kind: "motion_graphic", Provider: "Ollama", Model: "m:1b", Browser: "126.0", GeneratedSHA256: SHA256("<html>a</html>")}
	if m.Origin("<html>a</html>") != OriginGenerated || m.Origin("<html>b</html>") != OriginEdited {
		t.Fatal("a page the model wrote is generated until the text changes, then edited")
	}
	if (Meta{Kind: "overlay"}).Origin("x") != OriginSupplied {
		t.Fatal("a page nobody generated is supplied")
	}
	for _, want := range []string{"motion graphic", "m:1b", "Chrome 126.0"} {
		if d := m.Describe("<html>a</html>"); !strings.Contains(d, want) {
			t.Errorf("the summary must contain %q: %s", want, d)
		}
	}
	if d := m.Describe("<html>b</html>"); !strings.Contains(d, "then edited") {
		t.Errorf("an edited page says so: %s", d)
	}
}

func TestMetaRoundTripAndPaths(t *testing.T) {
	dir := t.TempDir()
	if _, ok := LoadMeta(dir, "s", false); ok {
		t.Fatal("nothing stored yet")
	}
	in := Meta{Kind: "overlay", Model: "m", Browser: "1.2"}
	if err := StoreMeta(dir, "s", true, in); err != nil {
		t.Fatal(err)
	}
	out, ok := LoadMeta(dir, "s", true)
	if !ok || out.Model != "m" || out.Kind != "overlay" {
		t.Fatalf("%+v %v", out, ok)
	}
	if MetaPath(dir, "s", true) == MetaPath(dir, "s", false) {
		t.Fatal("the overlay has its own metadata file")
	}
}

func TestPreviewHTMLPutsThePolicyAndTheClockBeforeThePageScripts(t *testing.T) {
	for _, page := range []string{
		`<!doctype html><html><head><title>x</title><script>var mine=1</script></head><body></body></html>`,
		`<html><body><script>var mine=1</script></body></html>`,
		`<div>fragment</div><script>var mine=1</script>`,
	} {
		out := PreviewHTML(page, 4.5)
		csp, clock, mine := strings.Index(out, "Content-Security-Policy"), strings.Index(out, "__motionSeek = function"), strings.Index(out, "var mine=1")
		if csp < 0 || clock < 0 || mine < 0 || csp >= clock || clock >= mine {
			t.Errorf("policy, then clock, then the page's script (%d %d %d): %s", csp, clock, mine, out)
		}
		if !strings.Contains(out, "window.__motionDuration = 4.5") || !strings.Contains(out, `"seek"`) {
			t.Errorf("the duration and the seek listener are set: %s", out)
		}
	}
}
