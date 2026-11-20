package musicmotion

import (
	"context"
	"strings"
	"testing"
)

const testManifest = `{"name":"probe","description":"a test part","version":1,"levels":["simple"],"params":[]}`

// A part that passes: a dot that swells inside every beat and moves on from beat to beat.
const movingFragment = `<style>#d{position:absolute;left:560px;top:200px;width:120px;height:120px;border-radius:50%;background:var(--accent)}</style><div id="d"></div>
<script>window.__partSeek=function(t,b){var k=Math.pow(1-b.position,2);var d=document.getElementById('d');d.style.transform='translateX('+(b.number*37%400)+'px) scale('+(1+k)+')';};</script>`

func parsed(t *testing.T, fragment string) Part {
	t.Helper()
	p, probs := ParsePart([]byte(testManifest), fragment)
	if len(probs) > 0 {
		t.Fatalf("%v", probs)
	}
	return p
}

func TestAGoodDraftPartPassesTheChecks(t *testing.T) {
	browser := browserOrSkip(t)
	probs, err := CheckPart(context.Background(), browser, parsed(t, movingFragment), CheckOptions{})
	if err != nil || len(probs) != 0 {
		t.Fatalf("%v %v", probs, err)
	}
}

func TestEachRuleOfAPartIsReportedWithItsReason(t *testing.T) {
	browser := browserOrSkip(t)
	cases := map[string]struct{ fragment, want string }{
		"static":   {`<div style="position:absolute;left:100px;top:100px;width:50px;height:50px;background:var(--accent)"></div><script>window.__partSeek=function(t,b){};</script>`, "does not move"},
		"random":   {`<div id="d" style="position:absolute;top:100px;width:50px;height:50px;background:var(--accent)"></div><script>window.__partSeek=function(t,b){document.getElementById('d').style.left=(Math.random()*900)+'px';};</script>`, "different frames"},
		"cut text": {`<div id="d" style="position:absolute;left:1180px;top:300px;font-size:90px;white-space:nowrap;color:var(--accent)">CUT BY THE EDGE</div><script>window.__partSeek=function(t,b){document.getElementById('d').style.top=(300+b.number*10+b.position*50)+'px';};</script>`, "cut by the right edge"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p, probs := ParsePart([]byte(testManifest), c.fragment)
			if len(probs) > 0 {
				t.Fatalf("%v", probs)
			}
			got, err := CheckPart(context.Background(), browser, p, CheckOptions{})
			if err != nil || len(got) == 0 || !strings.Contains(strings.Join(got, " "), c.want) {
				t.Fatalf("want a problem with %q: %v %v", c.want, got, err)
			}
		})
	}
}

func TestParsePartRefusesWhatTheCatalogRefuses(t *testing.T) {
	cases := map[string]struct{ manifest, fragment, want string }{
		"not json":   {`{`, movingFragment, "not valid JSON"},
		"no level":   {`{"name":"probe","description":"x","version":1,"levels":[],"params":[]}`, movingFragment, "no level"},
		"bad name":   {`{"name":"Bad Name","description":"x","version":1,"levels":["simple"],"params":[]}`, movingFragment, "name"},
		"no default": {`{"name":"probe","description":"x","version":1,"levels":["simple"],"params":[{"name":"a","type":"string"}]}`, movingFragment, "no default"},
		"empty":      {testManifest, "  ", "empty"},
		"external":   {testManifest, `<img src="https://example.com/a.png">` + movingFragment, "external"},
		"seek":       {testManifest, movingFragment + `<script>window.__seek = function(){}</script>`, "__seek"},
	}
	for name, c := range cases {
		_, probs := ParsePart([]byte(c.manifest), c.fragment)
		if len(probs) == 0 || !strings.Contains(strings.Join(probs, " "), c.want) {
			t.Errorf("%s: %v", name, probs)
		}
	}
}

func TestAPartCanBePreviewed(t *testing.T) {
	browser := browserOrSkip(t)
	frames, err := PreviewFrames(context.Background(), browser, parsed(t, movingFragment), 4)
	if err != nil || len(frames) != 4 || len(frames[0]) < 100 {
		t.Fatalf("%v %d", err, len(frames))
	}
	if string(frames[0]) == string(frames[3]) {
		t.Fatal("the preview moves over the beat")
	}
}

func TestTheAnswerOfTheAssistantIsReadAsTwoBlocks(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	good := "Here you go.\n```json\n" + testManifest + "\n```\nand the fragment:\n```html\n" + movingFragment + "\n```\n"
	m, f, p, probs := ParseDraftAnswer(good, cat)
	if len(probs) != 0 || p.Name != "probe" || !strings.Contains(string(m), "a test part") || !strings.Contains(f, "__partSeek") {
		t.Fatalf("%v %+v", probs, p)
	}
	cases := map[string]struct{ answer, want string }{
		"no manifest":  {"```html\n" + movingFragment + "\n```", "no ```json block"},
		"no fragment":  {"```json\n" + testManifest + "\n```", "no ```html block"},
		"nothing":      {"I cannot do that.", "no ```json block"},
		"taken name":   {"```json\n" + strings.Replace(testManifest, "probe", "toggle", 1) + "\n```\n```html\n" + movingFragment + "\n```", "already a part of the catalog"},
		"external":     {"```json\n" + testManifest + "\n```\n```html\n<img src=\"https://example.com/x.png\">" + movingFragment + "\n```", "external"},
		"bad manifest": {"```json\n{\"name\":\"probe\"}\n```\n```html\n" + movingFragment + "\n```", "description"},
	}
	for name, c := range cases {
		_, _, _, probs := ParseDraftAnswer(c.answer, cat)
		if len(probs) == 0 || !strings.Contains(strings.Join(probs, " "), c.want) {
			t.Errorf("%s: %v", name, probs)
		}
	}
}

func TestTheRequestCarriesTheSentenceTheLookTheNamesAndTheProblem(t *testing.T) {
	cat := Catalog{Builtin: BuiltinParts()}
	with := DraftUser(DraftInput{Request: "a gold clock that ticks", Description: "black and gold, thin lines", Cat: cat}, "it does not move")
	for _, want := range []string{"a gold clock that ticks", "black and gold, thin lines", "toggle", "EXAMPLE", "window.__partSeek", "it does not move"} {
		if !strings.Contains(with+DraftSystem(), want) {
			t.Errorf("missing %q", want)
		}
	}
	without := DraftUser(DraftInput{Request: "a gold clock", Cat: cat}, "")
	if !strings.Contains(without, "There is no picture") || strings.Contains(without, "previous answer") {
		t.Fatalf("%s", without)
	}
}
