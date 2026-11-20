package workspace

import (
	"strings"
	"testing"
)

func TestClientOf_StoredOrByKind(t *testing.T) {
	for name, tc := range map[string]struct {
		run          Run
		client, kind string
	}{
		"stored":           {Run{Project: GenPrefix + "x", Client: "video-studio"}, "video-studio", "video"},
		"edit stored":      {Run{Project: EditPrefix + "x", Client: "photo-lab"}, "photo-lab", "edit"},
		"old generation":   {Run{Project: GenPrefix + "x"}, "unknown", "video"},
		"old edit":         {Run{Project: EditPrefix + "x"}, "unknown", "edit"},
		"ordinary project": {Run{Project: "demo"}, "embedded", "project"},
		"mobile project":   {Run{Project: "mobile-1"}, "embedded", "project"},
		"deleted project":  {Run{Project: ""}, "embedded", "project"},
	} {
		if got := clientOf(tc.run); got != tc.client {
			t.Errorf("%s: client %q, want %q", name, got, tc.client)
		}
		if got := RunKindOf(tc.run.Project); got != tc.kind {
			t.Errorf("%s: kind %q, want %q", name, got, tc.kind)
		}
	}
}

// The run keeps the client its project's scenes.yaml names, and an ordinary project's run has none.
func TestLaunch_RunKeepsTheClientOfItsProject(t *testing.T) {
	s, fe := launchServer(t)
	raw, err := s.ProjectDefinition("demo")
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := launchRun(t, s, "demo", map[string]any{})
	fe.chanFor(plain) <- true
	waitState(t, s, plain, StateSucceeded)
	if err := s.SaveProjectDefinition("demo", []byte("client: video-studio\n"+strings.TrimSpace(string(raw))+"\n"), "save"); err != nil {
		t.Fatal(err)
	}
	tagged, _ := launchRun(t, s, "demo", map[string]any{})
	fe.chanFor(tagged) <- true
	waitState(t, s, tagged, StateSucceeded)
	if r, _ := s.rm.hist.getRun(plain); r.Client != "" {
		t.Errorf("an ordinary project's run has client %q", r.Client)
	}
	if r, _ := s.rm.hist.getRun(tagged); r.Client != "video-studio" {
		t.Errorf("the run has client %q, want video-studio", r.Client)
	}
}
