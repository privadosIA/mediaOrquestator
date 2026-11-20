package assist

import (
	"strings"
	"testing"
)

func TestParseStoryboard_ReadsTheLookAndEachShot(t *testing.T) {
	reply := "SUBJECT: A young woman with a red scarf, warm film look, golden hour\n" +
		"1: Wide shot of the woman walking a dusty road at sunrise, long shadows, 35mm, calm mood\n" +
		"2. Close-up of her hand letting go of a suitcase, soft backlight\n" +
		"Shot 3: She runs into a field of flowers, camera low, joyful"
	subject, prompts := ParseStoryboard(reply, 3, false)
	if !strings.Contains(subject, "red scarf") {
		t.Errorf("subject = %q", subject)
	}
	if len(prompts) != 3 || !strings.Contains(prompts[0], "dusty road") || !strings.Contains(prompts[1], "suitcase") || !strings.Contains(prompts[2], "field of flowers") {
		t.Errorf("prompts = %q", prompts)
	}
}

func TestParseStoryboard_MissingShotsStayEmptyAndNumbersOutOfRangeAreIgnored(t *testing.T) {
	_, prompts := ParseStoryboard("1: first picture of a quiet street at dawn with fog and a lone lamp post\n9: nonsense", 3, false)
	if prompts[0] == "" || prompts[1] != "" || prompts[2] != "" {
		t.Errorf("prompts = %q", prompts)
	}
}

func TestParseStoryboard_ATruncatedLastShotIsDropped(t *testing.T) {
	_, prompts := ParseStoryboard("1: a long enough description of a wide empty beach at dusk with gulls and soft waves rolling in\n2: she looks", 2, true)
	if prompts[0] == "" || prompts[1] != "" {
		t.Errorf("prompts = %q", prompts)
	}
}

func TestStoryboardUser_TellsTheSectionAndTheLinesOfEachShot(t *testing.T) {
	u := storyboardUser("lofi pop", "Noche", "a woman", []ShotBrief{{Section: "Verse", Lines: []string{"a", "b"}}, {Section: "Intro"}})
	for _, want := range []string{"Song: Noche", "Music style: lofi pop", "Look of the video (already decided): a woman", "1. [Verse] a / b", "2. [Intro] (no lyrics"} {
		if !strings.Contains(u, want) {
			t.Errorf("user message lacks %q:\n%s", want, u)
		}
	}
}
