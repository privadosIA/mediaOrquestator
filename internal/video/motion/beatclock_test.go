package motion

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

const beatSong = `{"bpm":120,"beats":[0,0.5,1,1.5,2,2.5,3,3.5,4],"sections":[{"tag":"Verse","start":0,"end":2},{"tag":"Chorus","start":2,"end":4}]}`

const beatPage = `<!doctype html><html><head><meta charset="utf-8"></head><body><div id="o"></div>
<script>window.__seek=function(t){ document.getElementById('o').textContent = JSON.stringify(window.__beat || null); };</script></body></html>`

type beatState struct {
	Number       int     `json:"number"`
	Position     float64 `json:"position"`
	Section      string  `json:"section"`
	SectionIndex int     `json:"sectionIndex"`
}

func beatAt(t *testing.T, s *Session, ctx context.Context, at float64) beatState {
	t.Helper()
	if _, err := s.Frame(ctx, at); err != nil {
		t.Fatal(err)
	}
	var b beatState
	if err := s.EvalJSON(ctx, "document.getElementById('o').textContent", &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBeatClock_NumberPositionAndSection(t *testing.T) {
	b := realBrowser(t)
	ctx := context.Background()
	s, err := Open(ctx, b, Page{HTML: beatPage, Width: 640, Height: 360, Duration: 4, Song: json.RawMessage(beatSong)}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if g := beatAt(t, s, ctx, 0.25); g.Number != 0 || math.Abs(g.Position-0.5) > 1e-6 || g.Section != "Verse" {
		t.Errorf("at 0.25 s: %+v", g)
	}
	if g := beatAt(t, s, ctx, 0.5); g.Number != 1 || g.Position > 1e-6 {
		t.Errorf("at 0.5 s the beat goes up by one: %+v", g)
	}
	if g := beatAt(t, s, ctx, 3.2); g.Number != 6 || math.Abs(g.Position-0.4) > 1e-6 || g.Section != "Chorus" || g.SectionIndex != 1 {
		t.Errorf("at 3.2 s: %+v", g)
	}
	if g := beatAt(t, s, ctx, 2.0); g.Section != "Chorus" {
		t.Errorf("the section changes at its start: %+v", g)
	}
}

func TestBeatClock_APageOutsideAPieceGetsNothing(t *testing.T) {
	b := realBrowser(t)
	ctx := context.Background()
	s, err := Open(ctx, b, Page{HTML: beatPage, Width: 640, Height: 360, Duration: 4}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Frame(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	if err := s.EvalJSON(ctx, "JSON.stringify([typeof window.__beat, typeof window.__motionSong])", &kinds); err != nil {
		t.Fatal(err)
	}
	if kinds[0] != "undefined" || kinds[1] != "undefined" {
		t.Errorf("a page outside a piece must see no beat clock and no song map, saw %v", kinds)
	}
}

func TestBeatClock_PreviewAndRenderInjectTheSameScript(t *testing.T) {
	song := json.RawMessage(beatSong)
	preview := PreviewHTMLSong(beatPage, 4, song)
	if !strings.Contains(preview, strings.ReplaceAll(ShimSong(4, song), "</script", "<\\/script")) {
		t.Fatal("the preview must carry the same clock script the renderer injects")
	}
	if PreviewHTMLSong(beatPage, 4, nil) != PreviewHTML(beatPage, 4) {
		t.Fatal("a page outside a piece must be previewed as before")
	}
}

// The page the documentation walks through must keep working: it renders with no problem and reacts to the beat.
func TestBeatClock_TheDocumentedExampleRuns(t *testing.T) {
	html, err := os.ReadFile("../../../docs/examples/beat-page.html")
	if err != nil {
		t.Fatal(err)
	}
	b := realBrowser(t)
	ctx := context.Background()
	s, err := Open(ctx, b, Page{HTML: string(html), Width: 1280, Height: 720, Duration: 4, Song: json.RawMessage(beatSong)}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scale := func(at float64) string {
		if _, err := s.Frame(ctx, at); err != nil {
			t.Fatal(err)
		}
		var v []string
		if err := s.EvalJSON(ctx, "JSON.stringify([document.getElementById('dot').style.transform, document.getElementById('tag').textContent])", &v); err != nil {
			t.Fatal(err)
		}
		return v[0] + "|" + v[1]
	}
	on, late := scale(1.0), scale(1.4)
	if on == late || !strings.HasSuffix(on, "|Verse") {
		t.Errorf("the dot must change inside a beat and name the section: on the beat %q, late %q", on, late)
	}
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("the example reported problems: %v", p)
	}
}
