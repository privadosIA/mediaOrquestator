package assist

import "testing"

func TestParseListenReply(t *testing.T) {
	g := ParseListenReply("STYLE=fast techno with synth and a male rap voice; THEME=una desplumadora de pollos que no para.")
	if g.Style != "fast techno with synth and a male rap voice" || g.Theme != "una desplumadora de pollos que no para" {
		t.Fatalf("%+v", g)
	}
	g = ParseListenReply("**STYLE:** slow ambient piano\nTHEME=")
	if g.Style != "slow ambient piano" || g.Theme != "" {
		t.Fatalf("%+v", g)
	}
	if g := ParseListenReply("no sé"); g.Style != "" || g.Theme != "" {
		t.Fatalf("an unreadable reply leaves it empty: %+v", g)
	}
}

func TestListenTaskIsRegisteredButNotOffered(t *testing.T) {
	if _, ok := tasks[TaskListen]; !ok {
		t.Fatal("the listen task is registered")
	}
	if _, err := ParseTask("listen"); err == nil {
		t.Fatal("the listen task must not be offered through ParseTask")
	}
}

func TestListenGuess_UnknownGenre(t *testing.T) {
	if g := ParseListenReply("STYLE=Unknown genre, dark steady texture; THEME="); !g.GenreUnknown() {
		t.Errorf("%+v", g)
	}
	if g := ParseListenReply("STYLE=fast techno; THEME="); g.GenreUnknown() {
		t.Errorf("%+v", g)
	}
}
