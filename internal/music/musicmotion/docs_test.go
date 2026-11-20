package musicmotion

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The examples in docs/MUSIC_MODE_MOTION.md are the ones a user copies: they must keep being valid.
func TestTheExamplesOfTheDocumentationAreValid(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/MUSIC_MODE_MOTION.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```json\\n(.*?)```").FindAllStringSubmatch(string(raw), -1)
	var seen []string
	for _, b := range blocks {
		text := b[1]
		switch {
		case strings.Contains(text, `"cost_ms"`):
			if _, err := parseManifest([]byte(text), "my-part"); err != nil {
				t.Errorf("the part manifest of the documentation: %v", err)
			}
			seen = append(seen, "part")
		case strings.Contains(text, `"default_pose"`):
			var m CharacterManifest
			if err := decodeStrict([]byte(text), &m); err != nil || m.Name == "" || m.Anchor.Y != 1 {
				t.Errorf("the character manifest of the documentation: %v %+v", err, m)
			}
			seen = append(seen, "character")
		case strings.Contains(text, `"stage"`):
			var wrapper struct {
				Stage Stage `json:"stage"`
			}
			if err := json.Unmarshal([]byte("{"+text+"}"), &wrapper); err != nil {
				t.Errorf("the stage of the documentation: %v", err)
				continue
			}
			if err := wrapper.Stage.Validate(lib(), 1280, 720); err != nil {
				t.Errorf("the stage of the documentation must be valid: %v", err)
			}
			seen = append(seen, "stage")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected a part, a character and a stage in the documentation, found %v", seen)
	}
}

// The rubric is one text: the writer's prompt, the reviewer's prompt and the documentation say the same, and the numbers the page
// checks are the ones the writer is told.
func TestTheRubricOfTheDocumentationIsTheOneOfThePrompts(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/MUSIC_MODE_MOTION.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, c := range RubricCriteria {
		if !strings.Contains(doc, c.Rule) {
			t.Errorf("the documentation lacks the rule of %s:\n%s", c.Key, c.Rule)
		}
		if !strings.Contains(SceneSystem(), c.Rule) || !strings.Contains(reviewSystem(), c.Question) {
			t.Errorf("a prompt lacks the criterion %s", c.Key)
		}
	}
	for _, n := range []string{"4 vh", "12 vh", "2 vh", "a quarter of the frame"} {
		if !strings.Contains(SceneSystem(), n) {
			t.Errorf("the writer is not told %q", n)
		}
	}
	if strings.Contains(RubricRules(), "small mono labels") || strings.Contains(RubricQuestions(), "small labels") {
		t.Error("the rubric still asks for labels")
	}
	if minVerseVh != 4 || minHeadlineVh != 12 || minTextVh != 2 || minPictureShare != 0.25 {
		t.Error("the numbers of the checks are not the ones the writer is told")
	}
}

// The craft rules are written once: the docs and the music-video skill list the same text between their craft markers.
func TestTheCraftRulesOfTheDocsAndOfTheSkillAreTheSameList(t *testing.T) {
	between := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		i, j := strings.Index(s, "<!-- craft:start -->"), strings.Index(s, "<!-- craft:end -->")
		if i < 0 || j < i {
			t.Fatalf("%s has no craft markers", path)
		}
		return strings.TrimSpace(s[i : j+len("<!-- craft:end -->")])
	}
	docs := between("../../../docs/MUSIC_MODE_MOTION.md")
	skill := between("../../../.claude/skills/music-video/references/craft.md")
	if docs != skill {
		t.Fatal("docs/MUSIC_MODE_MOTION.md § Craft and .claude/skills/music-video/references/craft.md differ")
	}
	if n := strings.Count(docs, "\n- *"); n < 15 {
		t.Fatalf("the craft list has %d rules, want the full list", n)
	}
}
