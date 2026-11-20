package motion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scripted is a Writer that answers from a list and records what it was asked.
type scripted struct {
	answers []string
	systems []string
	users   []string
}

func (s *scripted) Write(_ context.Context, system, user string, _ int) (string, error) {
	s.systems = append(s.systems, system)
	s.users = append(s.users, user)
	i := len(s.users) - 1
	if i >= len(s.answers) {
		i = len(s.answers) - 1
	}
	return s.answers[i], nil
}

func req() Request {
	return Request{Description: "three bullets appearing", Narration: "Tres ideas clave", Style: "flat, dark blue", Width: 1280, Height: 720, Duration: 4.5}
}

func TestPromptCarriesWhatThePageNeeds(t *testing.T) {
	w := &scripted{answers: []string{slidePage}}
	r := req()
	if _, err := Generate(context.Background(), w, "", r, Limits{}); err == nil {
		// no browser given: the static check passes and Open fails; either way the request was made
		t.Log("generated")
	}
	u := w.users[0]
	for _, want := range []string{"1280x720", "4.50 seconds", "three bullets appearing", "Tres ideas clave", "flat, dark blue", "never write this text on the screen", "FULL SCENE"} {
		if !strings.Contains(u, want) {
			t.Errorf("the request must carry %q:\n%s", want, u)
		}
	}
	for _, want := range []string{"offline", "__seek", "system font", "exactly the words", "position:relative", "motion-timeline", "NEVER copy", "constant slow speed", "three kinds of motion"} {
		if !strings.Contains(w.systems[0], want) {
			t.Errorf("the system prompt must state %q", want)
		}
	}
	w = &scripted{answers: []string{slidePage}}
	r.Overlay, r.Position = true, "top"
	Generate(context.Background(), w, "", r, Limits{})
	u = w.users[0]
	if !strings.Contains(u, "OVERLAY") || !strings.Contains(u, "transparent") || !strings.Contains(u, "top of the frame") {
		t.Errorf("an overlay request must demand transparency and the position:\n%s", u)
	}
	if !strings.Contains(u, "Draw ONLY this") || strings.Contains(u, "flat, dark blue") || strings.Contains(u, "Tres ideas clave") {
		t.Errorf("an overlay shows its own text only: no style and no narration to tempt the model:\n%s", u)
	}
}

func TestExtractHTML(t *testing.T) {
	cases := map[string]string{
		"```html\n<!doctype html><html><body>x</body></html>\n```": "<!doctype html><html><body>x</body></html>",
		"Here you go:\n<html><body>y</body></html>\nEnjoy":         "<html><body>y</body></html>",
		"  <!DOCTYPE html><html></html>  ":                         "<!DOCTYPE html><html></html>",
	}
	for in, want := range cases {
		if got := ExtractHTML(in); got != want {
			t.Errorf("ExtractHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStaticProblems(t *testing.T) {
	bad := map[string]string{
		`<html><body><script src="https://cdn.example.com/gsap.js"></script></body></html>`:       "external",
		`<html><body><img src="//example.com/a.png"></body></html>`:                               "external",
		`<html><head><style>@import url("https://fonts.googleapis.com/x");</style></head></html>`: "external",
		`<html><head><link rel="stylesheet" href="a.css"></head></html>`:                          "external",
		`<html><body style="background:url(http://x/y.png)"></body></html>`:                       "external",
		`just text`: "complete HTML document",
	}
	for html, want := range bad {
		p := strings.Join(StaticProblems(html), ";")
		if !strings.Contains(p, want) {
			t.Errorf("%q: got %q, want %q", html, p, want)
		}
	}
	if p := StaticProblems(slidePage); len(p) != 0 {
		t.Errorf("a clean page has no static problems: %v", p)
	}
	if p := StaticProblems(`<html><body><a href="#top">x</a><svg><use href="#a"/></svg></body></html>`); len(p) != 0 {
		t.Errorf("local references are fine: %v", p)
	}
}

func TestCheckRejectionsThatNeedTheBrowser(t *testing.T) {
	b := realBrowser(t)
	lim := Limits{}
	cases := []struct {
		name, html string
		overlay    bool
		want       string
	}{
		{"static", `<!doctype html><html><body style="background:#123"><h1>hi</h1></body></html>`, false, "does not change"},
		{"throws", `<!doctype html><html><body><script>window.__seek=function(t){ if(t>1) throw new Error("kaput"); };</script></body></html>`, false, "kaput"},
		{"blank overlay", `<!doctype html><html><body></body></html>`, true, "draws nothing"},
		{"overlay with a background", `<!doctype html><html><body style="margin:0;background:#123"><h1 style="color:#fff">Title</h1></body></html>`, true, "covers"},
		{"remote image", `<!doctype html><html><body><div style="background:#f00;width:10px;height:10px"></div><script>var i=new Image();i.src="htt"+"p://example.com/a.png";document.body.appendChild(i)</script></body></html>`, false, "outside itself"},
	}
	for _, c := range cases {
		probs, err := Check(context.Background(), b, Page{HTML: c.html, Width: 200, Height: 80, Duration: 2, Transparent: c.overlay}, lim)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(strings.Join(probs, ";"), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, probs, c.want)
		}
	}
	probs, err := Check(context.Background(), b, Page{HTML: slidePage, Width: 200, Height: 80, Duration: 2}, lim)
	if err != nil || len(probs) != 0 {
		t.Fatalf("a good page must pass: %v %v", probs, err)
	}
}

func TestGenerateRepairsWithTheProblemsFound(t *testing.T) {
	b := realBrowser(t)
	w := &scripted{answers: []string{
		`<html><body><script src="https://cdn.example.com/x.js"></script></body></html>`,
		"```html\n" + slidePage + "\n```",
	}}
	r := req()
	r.Width, r.Height = 200, 80
	r.Duration = 2
	html, err := Generate(context.Background(), w, b, r, Limits{})
	if err != nil || html != slidePage {
		t.Fatalf("got %v / %q", err, html)
	}
	if len(w.users) != 2 || !strings.Contains(w.users[1], "external resource") || !strings.Contains(w.users[1], "cdn.example.com") {
		t.Fatalf("the second request must carry the problem and the previous page: %v", w.users)
	}
}

func TestGenerateGivesUpAfterThreeAttemptsNamingTheLastProblem(t *testing.T) {
	w := &scripted{answers: []string{`<html><body><img src="http://x.test/a.png"></body></html>`}}
	_, err := Generate(context.Background(), w, "", req(), Limits{})
	if !errors.Is(err, ErrPageRejected) || !strings.Contains(err.Error(), "external resource") || !strings.Contains(err.Error(), "3 attempts") {
		t.Fatalf("got %v", err)
	}
	if len(w.users) != MaxAttempts {
		t.Fatalf("the model must be asked %d times, was %d", MaxAttempts, len(w.users))
	}
}

func TestEnsureStoresOnceAndHonoursEdits(t *testing.T) {
	b := realBrowser(t)
	dir := t.TempDir()
	w := &scripted{answers: []string{slidePage}}
	r := Request{Description: "x", Width: 200, Height: 80, Duration: 2}
	html, gen, err := Ensure(context.Background(), w, b, dir, "s1", r, Limits{})
	if err != nil || !gen || html != slidePage {
		t.Fatalf("first: %v %v", gen, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "motion", "s1.html")); string(data) != slidePage {
		t.Fatal("the page must be stored under motion/")
	}
	if _, gen, _ = Ensure(context.Background(), w, b, dir, "s1", r, Limits{}); gen || len(w.users) != 1 {
		t.Fatal("the second run must not call the model")
	}
	edited := strings.ReplaceAll(slidePage, "#0033aa", "#00aa33")
	if err := os.WriteFile(filepath.Join(dir, "motion", "s1.html"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	html, gen, _ = Ensure(context.Background(), w, b, dir, "s1", r, Limits{})
	if gen || html != edited || len(w.users) != 1 {
		t.Fatal("an edited page must be used as it is, without calling the model")
	}
	// the overlay of the same scene is a different file
	if PagePath(dir, "s1", true) == PagePath(dir, "s1", false) {
		t.Fatal("scene page and overlay page must not share a file")
	}
}

// The example in the instructions carries placeholders, not words a model could copy onto the screen.
func TestSystemPromptExampleHasNoCopyableWords(t *testing.T) {
	for _, bad := range []string{"Example heading", "Episode 1", "My first video"} {
		if strings.Contains(systemPrompt, bad) {
			t.Errorf("the example must not contain %q", bad)
		}
	}
	if !strings.Contains(systemPrompt, "[HEADING]") || !strings.Contains(systemPrompt, "placeholders") {
		t.Error("the example must use bracketed placeholders and say so")
	}
}

// The instructions name the timeline block, every property and every curve the script accepts.
func TestSystemPromptDescribesTheTimeline(t *testing.T) {
	for _, want := range append(append([]string{`id="motion-timeline"`, `"version":1`, "stagger", "BOTH from and to"}, TimelineProperties...), TimelineCurves...) {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("the instructions must mention %q", want)
		}
	}
	if strings.Count(systemPrompt, `id="motion-timeline"`) < 2 {
		t.Error("the example page must carry a timeline block")
	}
}
