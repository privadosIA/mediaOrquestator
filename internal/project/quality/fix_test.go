package quality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixYAML = `# a project
project: demo
defaults:
  animate: {frames: 33}   # keep this comment
scenes:
  - id: a            # first
    prompt: one
    seed: 7
    silence_seconds: 2
    animate: true
  - id: b
    prompt: two
    silence_seconds: 2
  - id: c
    prompt: three
    tts_text: Hello
`

func TestApplyFixes_EditsOnlyWhatIsAskedKeepingTheComments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scenes.yaml")
	os.WriteFile(p, []byte(fixYAML), 0o644)
	cut, newSeed, other := 1.2, 1007, 5
	if err := ApplyFixes(p, []Fix{{ID: "a", SilenceSeconds: &cut, Seed: &newSeed}, {ID: "b", Seed: &other}}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	for _, want := range []string{"# a project", "# keep this comment", "# first", "silence_seconds: 1.2", "seed: 1007", "seed: 5"} {
		if !strings.Contains(s, want) {
			t.Errorf("the file lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "seed: 7") || strings.Count(s, "silence_seconds") != 2 {
		t.Errorf("the old values must be replaced, not added:\n%s", s)
	}
	// the untouched scene is as it was
	if !strings.Contains(s, "tts_text: Hello") {
		t.Errorf("scene c changed:\n%s", s)
	}
	// the original is kept once, and a second edit does not overwrite it
	bak, _ := os.ReadFile(p + ".bak")
	if string(bak) != fixYAML {
		t.Errorf("the backup must be the original file:\n%s", bak)
	}
	again := 2.0
	if err := ApplyFixes(p, []Fix{{ID: "a", SilenceSeconds: &again}}); err != nil {
		t.Fatal(err)
	}
	if bak2, _ := os.ReadFile(p + ".bak"); string(bak2) != fixYAML {
		t.Error("the first backup must survive a second edit")
	}
}

func TestApplyFixes_UnknownSceneWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scenes.yaml")
	os.WriteFile(p, []byte(fixYAML), 0o644)
	cut := 1.0
	if err := ApplyFixes(p, []Fix{{ID: "a", SilenceSeconds: &cut}, {ID: "nope", SilenceSeconds: &cut}}); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != fixYAML {
		t.Error("the file must not change when a fix is refused")
	}
	if _, err := os.Stat(p + ".bak"); err == nil {
		t.Error("no backup without a write")
	}
}
