package importer

import (
	"strings"
	"testing"
)

func settingsEntries() []Entry {
	return []Entry{
		{ID: "talk", FullPrompt: "a bench\nDialogue: \"Hello\"\nSEED: 7\nspeed: 1.2\nSilence: 5\nVariants: 3"},
		{ID: "quiet", FullPrompt: "the park\nsilence: 3\nSpeed: 2\nMotion: leaves move in the wind, static camera"},
		{ID: "bad", FullPrompt: "x\nSeed: abc"},
		{ID: "bad2", FullPrompt: "x\nMotion:"},
	}
}

func TestMap_SceneSettingsLines(t *testing.T) {
	res, err := Map(settingsEntries(), nil, Options{Project: "p", SilenceSeconds: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Rejected != 2 {
		t.Fatalf("imported %d rejected %d: %s", res.Imported, res.Rejected, res.Report())
	}
	talk, quiet := res.Config.Scenes[0], res.Config.Scenes[1]
	if talk.Prompt != "a bench" || talk.TTSText != "Hello" || talk.Seed == nil || *talk.Seed != 7 || talk.Speed != 1.2 || talk.Variants == nil || *talk.Variants != 3 || talk.SilenceSeconds != nil {
		t.Errorf("talk = %+v", talk)
	}
	if quiet.SilenceSeconds == nil || *quiet.SilenceSeconds != 3 || !quiet.Animate || quiet.MotionPrompt != "leaves move in the wind, static camera" || quiet.Speed != 0 {
		t.Errorf("quiet = %+v", quiet)
	}
	report := res.Report()
	for _, want := range []string{"Silence 5: the scene has a dialogue", "Speed 2: the scene is silent", `Seed "abc" is not a non-negative integer`, "Motion line without text", "motion_prompt ←"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
}

func TestEmitYAML_WritesTheSettingsAndTheAnimatePlaceholder(t *testing.T) {
	res, _ := Map(settingsEntries()[:2], nil, Options{Project: "p", SilenceSeconds: 2})
	yaml := string(res.EmitYAML())
	for _, want := range []string{"seed: 7", "variants: 3", "speed: 1.2", "silence_seconds: 3", "animate: true", "motion_prompt: leaves move in the wind, static camera", "# animate:", "#   diffusion_model: models/video/"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("the YAML lacks %q:\n%s", want, yaml)
		}
	}
	// without a moving scene there is no animate block, and a plain scene keeps the old speed line
	plain, _ := Map([]Entry{{ID: "a", FullPrompt: "x\nDialogue: \"Hi\""}}, nil, Options{Project: "p", SilenceSeconds: 2})
	py := string(plain.EmitYAML())
	if strings.Contains(py, "animate") || !strings.Contains(py, "speed: 1.0") {
		t.Errorf("plain YAML:\n%s", py)
	}
}
