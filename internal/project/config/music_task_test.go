package config

import (
	"os"
	"path/filepath"
	"testing"
)

// taskProject returns a project directory holding sources/base.mp3.
func taskProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, SourcesSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SourcesSubdir, "base.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func f(v float64) *float64 { return &v }

func TestTask_ValidTracks(t *testing.T) {
	dir := taskProject(t)
	for name, tr := range map[string]Track{
		"cover":       {ID: "a", Prompt: "jazz", Task: TaskCover, Source: "base.mp3", CoverStrength: f(0.6)},
		"cover-nofsq": {ID: "a", Prompt: "jazz", Task: TaskCoverNoFSQ, Source: "base.mp3"},
		"repaint":     {ID: "a", Prompt: "jazz", Task: TaskRepaint, Source: "base.mp3", RepaintStart: f(10), RepaintEnd: f(25)},
		"outpaint":    {ID: "a", Prompt: "jazz", Task: TaskRepaint, Source: "base.mp3", RepaintStart: f(-5)},
		"extract":     {ID: "a", Prompt: "voice", Task: TaskExtract, Source: "base.mp3", Stem: "vocals"},
	} {
		c := musicCfg(tr)
		if name == "extract" {
			c.Defaults.Music.SynthModel = "acestep-v15-sft-Q8_0.gguf"
			if err := os.WriteFile(filepath.Join(c.Defaults.Music.ModelsDir, "acestep-v15-sft-Q8_0.gguf"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := c.ValidateInDir(linked(dir)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTask_Rejections(t *testing.T) {
	dir := taskProject(t)
	cases := map[string]struct {
		tr   Track
		want string
	}{
		"missing source":      {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "nope.mp3"}, `source "nope.mp3" not found`},
		"no source":           {Track{ID: "a", Prompt: "p", Task: TaskCover}, "needs a source"},
		"path":                {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "../x.mp3"}, "not a path"},
		"folder":              {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "a/b.mp3"}, "not a path"},
		"not audio":           {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.ogg"}, "mp3 or .wav"},
		"source no task":      {Track{ID: "a", Prompt: "p", Source: "base.mp3"}, "source needs a task"},
		"unknown task":        {Track{ID: "a", Prompt: "p", Task: "remix", Source: "base.mp3"}, `task "remix" is invalid`},
		"lego":                {Track{ID: "a", Prompt: "p", Task: "lego", Source: "base.mp3"}, "not supported"},
		"strength range":      {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", CoverStrength: f(1.5)}, "between 0 and 1"},
		"strength on repaint": {Track{ID: "a", Prompt: "p", Task: TaskRepaint, Source: "base.mp3", CoverStrength: f(0.5)}, "cover_strength belongs to cover"},
		"region on cover":     {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", RepaintStart: f(1)}, "belong to repaint"},
		"inverted region":     {Track{ID: "a", Prompt: "p", Task: TaskRepaint, Source: "base.mp3", RepaintStart: f(20), RepaintEnd: f(10)}, "must be greater than repaint_start"},
		"extract no stem":     {Track{ID: "a", Prompt: "p", Task: TaskExtract, Source: "base.mp3"}, "needs a stem"},
		"bad stem":            {Track{ID: "a", Prompt: "p", Task: TaskExtract, Source: "base.mp3", Stem: "kazoo"}, `stem "kazoo" is invalid`},
		"stem on cover":       {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", Stem: "vocals"}, "stem belongs to extract"},
		"duration":            {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", DurationSeconds: f(30)}, "source decides the length"},
		"extract on turbo":    {Track{ID: "a", Prompt: "p", Task: TaskExtract, Source: "base.mp3", Stem: "drums"}, "install the SFT model"},
	}
	for name, tc := range cases {
		c := musicCfg(tc.tr)
		if name == "extract on turbo" {
			c.Defaults.Music.SynthModel = "acestep-v15-turbo-Q8_0.gguf"
		}
		err := c.ValidateInDir(linked(dir))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		wantErr(t, err, tc.want)
	}
}

func TestTask_TaskTracksNeverUseTheLanguageModel(t *testing.T) {
	use := true
	tr := Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", UseLM: &use}
	if tr.EffectiveUseLM() {
		t.Fatal("a task track must not compose")
	}
	c := musicCfg(tr)
	if c.UsesLM() {
		t.Fatal("a project of task tracks does not need the language model")
	}
	if err := c.ValidateInDir(linked(taskProject(t))); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range c.Warnings {
		if len(w) > 0 && w == `track "a": use_lm is ignored because the track has a task` {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v", c.Warnings)
	}
}

func TestTask_YAMLRoundTrip(t *testing.T) {
	c, err := LoadBytes([]byte("project: p\noutput: {mode: music}\ntracks:\n  - {id: a, prompt: jazz, task: repaint, source: base.mp3, repaint_start: 10, repaint_end: 25.5}\n"))
	if err != nil {
		t.Fatal(err)
	}
	tr := c.Tracks[0]
	if tr.Task != TaskRepaint || tr.Source != "base.mp3" || *tr.RepaintStart != 10 || *tr.RepaintEnd != 25.5 {
		t.Fatalf("%+v", tr)
	}
}

func TestTask_ValidSourceName(t *testing.T) {
	for n, want := range map[string]bool{"a.mp3": true, "A-b_c.WAV": true, "a.ogg": false, "a/b.mp3": false, "../a.mp3": false, ".a.mp3": false, "": false, "a b.mp3": false} {
		if got := ValidSourceName(n); got != want {
			t.Errorf("%q: %v", n, got)
		}
	}
}

func TestReference_ValidAndRejections(t *testing.T) {
	dir := taskProject(t)
	ok := musicCfg(Track{ID: "a", Prompt: "p", Reference: "base.mp3"})
	if err := ok.ValidateInDir(linked(dir)); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		tr   Track
		want string
	}{
		"missing":   {Track{ID: "a", Prompt: "p", Reference: "nope.mp3"}, `reference "nope.mp3" not found`},
		"path":      {Track{ID: "a", Prompt: "p", Reference: "../x.mp3"}, "not a path"},
		"not audio": {Track{ID: "a", Prompt: "p", Reference: "x.ogg"}, "mp3 or .wav"},
		"with task": {Track{ID: "a", Prompt: "p", Task: TaskCover, Source: "base.mp3", Reference: "base.mp3"}, "reference is for new songs"},
	} {
		wantErr(t, musicCfg(tc.tr).ValidateInDir(linked(dir)), tc.want)
		_ = name
	}
}
