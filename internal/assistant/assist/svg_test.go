package assist

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><g id="logo"><circle id="ring" cx="960" cy="300" r="120" fill="none" stroke="#fff" stroke-width="8"/><rect id="bar-1" x="100" y="900" width="300" height="40" fill="#0af"/></g></svg>`

func TestExtractSVG_TakesTheDocumentOutOfTheReply(t *testing.T) {
	for name, reply := range map[string]string{
		"bare":       goodSVG,
		"fenced":     "```svg\n" + goodSVG + "\n```",
		"with words": "Here is your drawing:\n" + goodSVG + "\nHope it helps!",
		"reasoning":  "<think>a circle, maybe <svg></think>" + goodSVG,
	} {
		got, err := ExtractSVG(reply)
		if err != nil || got != goodSVG {
			t.Errorf("%s: %v %q", name, err, got)
		}
	}
}

func TestExtractSVG_RefusesWhatIsNotAUsableSVG(t *testing.T) {
	wrap := func(inner string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` + inner + `</svg>`
	}
	for name, c := range map[string]struct{ reply, why string }{
		"no svg":          {"I would draw a circle.", "no <svg>"},
		"not closed":      {`<svg viewBox="0 0 1 1"><g>`, "does not end"},
		"not well formed": {`<svg viewBox="0 0 1 1"><g></svg>`, "well-formed"},
		"no viewbox":      {`<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`, "viewBox"},
		"empty viewbox":   {`<svg viewBox=" "><g/></svg>`, "viewBox"},
		"script":          {wrap(`<script>alert(1)</script>`), "<script>"},
		"foreignObject":   {wrap(`<foreignObject><div/></foreignObject>`), "foreignObject"},
		"event attribute": {wrap(`<circle r="1" onload="x()"/>`), "onload"},
		"outside href":    {wrap(`<image href="https://example.com/a.png"/>`), "outside"},
		"outside xlink":   {wrap(`<use xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="http://x/y.svg#a"/>`), "outside"},
		"too long":        {wrap(strings.Repeat(`<g/>`, MaxSVGBytes/4)), "KB"},
	} {
		_, err := ExtractSVG(c.reply)
		if !errors.Is(err, ErrBadSVG) || !strings.Contains(err.Error(), c.why) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := ExtractSVG(wrap(`<use href="#a"/>`)); err != nil {
		t.Errorf("an inside reference is fine: %v", err)
	}
}

func TestSVG_AsksForTheDrawingOnTheCPUAndReturnsIt(t *testing.T) {
	f := &fakeOllama{answer: "```svg\n" + goodSVG + "\n```"}
	a := setup(t, f)
	got, err := a.SVG(context.Background(), "un círculo con tres barras debajo")
	if err != nil || got != goodSVG {
		t.Fatalf("%v %q", err, got)
	}
	req := f.chats[0]
	opts := req["options"].(map[string]any)
	if opts["num_gpu"] != float64(0) || opts["num_predict"] != float64(svgMaxTokens) {
		t.Errorf("options = %v", opts)
	}
	msgs := req["messages"].([]any)
	sys := msgs[0].(map[string]any)["content"].(string)
	usr := msgs[1].(map[string]any)["content"].(string)
	for _, want := range []string{"viewBox", "id to every element", "No <script>", "No <text>"} {
		if !strings.Contains(sys, want) {
			t.Errorf("the prompt lacks %q: %s", want, sys)
		}
	}
	if !strings.Contains(usr, "un círculo con tres barras") {
		t.Errorf("user prompt = %q", usr)
	}
	if f.unloadCount() == 0 {
		t.Error("the model was not unloaded")
	}
}

func TestSVG_FailuresAreTheRightKind(t *testing.T) {
	ctx := context.Background()
	if _, err := setup(t, &fakeOllama{answer: "Lo siento, no puedo dibujar eso."}).SVG(ctx, "algo"); !errors.Is(err, ErrRefused) {
		t.Errorf("a refusal: %v", err)
	}
	if _, err := setup(t, &fakeOllama{answer: `<svg viewBox="0 0 1 1"><script/></svg>`}).SVG(ctx, "algo"); !errors.Is(err, ErrBadSVG) {
		t.Errorf("a script: %v", err)
	}
	if _, err := setup(t, &fakeOllama{answer: goodSVG, doneRsn: "length"}).SVG(ctx, "algo"); !errors.Is(err, ErrBadSVG) || !strings.Contains(err.Error(), "cut off") {
		t.Errorf("cut off: %v", err)
	}
	if _, err := setup(t, &fakeOllama{answer: goodSVG}).SVG(ctx, "  "); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty: %v", err)
	}
	f := &fakeOllama{models: []string{"other:1b"}, answer: goodSVG}
	if _, err := setup(t, f).SVG(ctx, "algo"); !errors.Is(err, ErrNotInstalled) || len(f.chats) != 0 {
		t.Errorf("not installed: %v", err)
	}
}

func TestTheSVGTaskIsNotOfferedThroughParseTask(t *testing.T) {
	if _, err := ParseTask(string(TaskSVG)); err == nil {
		t.Fatal("svg goes through SVG, not through POST /api/assist")
	}
}
