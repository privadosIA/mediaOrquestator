package assist

import (
	"errors"
	"testing"
)

func TestParseKindReply(t *testing.T) {
	g := ParseKindReply("KIND=music; LANG=es; TITLE=Despedida en la estación; INSTRUMENTAL=yes; SECONDS=120")
	if !g.Understood || g.Kind != KindMusic || g.Language != "es" || g.Title != "Despedida en la estación" || !g.Instrumental || g.Seconds != 120 {
		t.Fatalf("%+v", g)
	}
	// A model that dresses the line up: reasoning, bold, a colon instead of an equals sign.
	g = ParseKindReply("<think>hmm</think> **KIND:** Image; lang: EN; title: \"A cat on a roof.\"; INSTRUMENTAL: no; SECONDS: 0")
	if g.Kind != KindImage || g.Language != "en" || g.Title != "A cat on a roof" || g.Instrumental || g.Seconds != 0 {
		t.Fatalf("%+v", g)
	}
}

func TestParseKindReply_NotUnderstood(t *testing.T) {
	for _, reply := range []string{"", "I think this is a nice request", "KIND=poem; TITLE=x"} {
		if g := ParseKindReply(reply); g.Understood || g.Kind != "" {
			t.Errorf("%q: %+v", reply, g)
		}
	}
}

func TestParseKind(t *testing.T) {
	if k, err := ParseKind(" Video "); err != nil || k != KindVideo {
		t.Errorf("%v %v", k, err)
	}
	if _, err := ParseKind("poem"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("%v", err)
	}
}

func TestTitleFrom(t *testing.T) {
	if got := TitleFrom("Hazme un vídeo de un gato en una azotea al atardecer."); got != "Hazme un vídeo de un gato" {
		t.Errorf("%q", got)
	}
	if got := TitleFrom("  una canción triste. "); got != "una canción triste" {
		t.Errorf("%q", got)
	}
}

func TestTheKindTaskIsNotOfferedThroughParseTask(t *testing.T) {
	if _, err := ParseTask(string(TaskKind)); err == nil {
		t.Fatal("kind must stay internal to the request planner")
	}
	if _, err := ParseTask("image"); err != nil {
		t.Fatal(err)
	}
}

func TestSplitSpeech(t *testing.T) {
	cases := []struct{ in, speech, rest string }{
		{"una mujer en la playa que dice «hola, bienvenidos»", "hola, bienvenidos", "una mujer en la playa"},
		{`a man in a kitchen saying "good morning everyone" while he cooks`, "good morning everyone", "a man in a kitchen while he cooks"},
		{"una mujer en la playa que diga: bienvenidos a mi canal", "bienvenidos a mi canal", "una mujer en la playa"},
		{"a woman waves and says hello there!", "hello there", "a woman waves"},
		{"un gato en una azotea al atardecer", "", "un gato en una azotea al atardecer"},
		{"dice \"\"", "", `dice ""`},
		{"“uno” y luego “dos” en un coche", "uno dos", "y luego en un coche"},
	}
	for _, c := range cases {
		speech, rest := SplitSpeech(c.in)
		if speech != c.speech || rest != c.rest {
			t.Errorf("SplitSpeech(%q) = %q, %q; want %q, %q", c.in, speech, rest, c.speech, c.rest)
		}
	}
}
