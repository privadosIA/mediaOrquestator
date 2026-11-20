package assist

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const heartReply = "STYLE: Warm acoustic wedding song with piano and a soft female voice, romantic, slow\n" +
	"TAGS: Piano, Acoustic, Wedding, Romantic.\n" +
	"LYRICS:\n[Intro]\n[Verse 1]\nHoy te digo que sí\n[Pre-Chorus]\nEs para siempre\n[Chorus:]\nContigo hasta el final\n[Outro 2]\n[Solo]"

func TestParseEngine(t *testing.T) {
	for _, ok := range []string{"", "ace-step", "heartmula", "diffrhythm2"} {
		if _, err := ParseEngine(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if _, err := ParseEngine("suno"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("an unknown engine: %v", err)
	}
}

func TestEngineSections_KeepsWhatTheEnginesReadInLowerCase(t *testing.T) {
	got := strings.Join(EngineSections(strings.Split("[Intro]\n[Verse 1]\nhello\n[Pre-Chorus]\nthere\n[Chorus:]\nyes\n[Outro 2]\n[Solo]\n[Foo]", "\n")), "|")
	if got != "[intro]|[verse]|hello|there|[chorus]|yes|[outro]|[solo]" {
		t.Fatalf("got %q", got)
	}
}

func TestParseMusicFor_ReadsTagsAndWritesTheSectionsOfTheEngine(t *testing.T) {
	got, err := ParseMusicFor(heartReply, false, false, EngineHeartMuLa)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags != "romantic,piano,acoustic" || !got.NoGenre {
		t.Errorf("tags = %q", got.Tags)
	}
	if got.Lyrics != "[intro]\n[verse]\nHoy te digo que sí\nEs para siempre\n[chorus]\nContigo hasta el final\n[outro]\n[solo]" {
		t.Errorf("lyrics = %q", got.Lyrics)
	}
	if !strings.HasPrefix(got.Caption, "Warm acoustic wedding song") {
		t.Errorf("caption = %q", got.Caption)
	}
}

func TestParseMusicFor_ACEStepIsUnchangedAndHasNoTags(t *testing.T) {
	got, err := ParseMusicFor(heartReply, false, false, EngineACEStep)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags != "" || !strings.Contains(got.Lyrics, "[Verse 1]") || !strings.Contains(got.Lyrics, "[Pre-Chorus]") {
		t.Fatalf("%+v", got)
	}
	// and the old entry point is the same thing
	old, _ := ParseMusic(heartReply, false, false)
	if old != got {
		t.Fatalf("ParseMusic differs: %+v vs %+v", old, got)
	}
}

func TestParseMusicFor_ATagsLineIsNotRequired(t *testing.T) {
	got, err := ParseMusicFor("STYLE: Calm piano\nLYRICS:\n[Verse]\nhola", false, false, EngineDiffRhythm2)
	if err != nil || got.Tags != "" || got.Lyrics != "[verse]\nhola" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestMusicFor_AsksHeartMuLaForTagsAndItsSections(t *testing.T) {
	f := &fakeOllama{answer: heartReply}
	a := setup(t, f)
	got, err := a.MusicFor(context.Background(), "Una canción de boda", MusicOptions{Engine: EngineHeartMuLa})
	if err != nil || got.Tags == "" {
		t.Fatalf("%+v %v", got, err)
	}
	sys := f.chats[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	for _, want := range []string{"TAGS:", "[intro]", "[inst]", "[solo]", "lower case"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt lacks %q: %q", want, sys)
		}
	}
	if _, err := a.MusicFor(context.Background(), "x", MusicOptions{Engine: "suno"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("unknown engine: %v", err)
	}
}

const transcript = "Camino por la calle\nbajo la lluvia\nVuelvo a casa\nvuelvo a ti\nVuelvo a casa\nvuelvo a ti"

func TestLyricsFromTranscript_KeepsTheWordsAndPutsTheSections(t *testing.T) {
	reply := "STYLE: Slow Spanish ballad, melancholic, sung by a man\nTAGS: ballad,melancholic,slow,male vocals\nLYRICS:\n[Verse]\nCamino por la calle\nbajo la lluvia\n[Chorus]\nVuelvo a casa\nvuelvo a ti\n[Chorus]\nVuelvo a casa\nvuelvo a ti"
	f := &fakeOllama{answer: reply}
	a := setup(t, f)
	d, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: transcript, Language: "es", Engine: EngineHeartMuLa, BPM: 71, Key: "F# major"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.UnchangedWords || !strings.HasPrefix(d.Lyrics, "[verse]\nCamino por la calle") || d.Tags == "" {
		t.Fatalf("%+v", d)
	}
	usr := f.chats[0]["messages"].([]any)[1].(map[string]any)["content"].(string)
	for _, want := range []string{"Language of the lyrics: es", "about 71 bpm", "F# major", "Camino por la calle"} {
		if !strings.Contains(usr, want) {
			t.Errorf("user prompt lacks %q: %q", want, usr)
		}
	}
	sys := f.chats[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "never add") {
		t.Errorf("the rules against inventing are missing: %q", sys)
	}
}

func TestLyricsFromTranscript_AModelThatRewritesFallsBackToWhatWasHeard(t *testing.T) {
	reply := "STYLE: Ballad\nLYRICS:\n[Verse]\nCamino por la calle\nbajo la tormenta\n[Chorus]\nVuelvo a casa mi amor"
	a := setup(t, &fakeOllama{answer: reply})
	d, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: transcript, Language: "es"})
	if err != nil {
		t.Fatal(err)
	}
	if d.UnchangedWords || d.Lyrics != "[Verse]\n"+transcript {
		t.Fatalf("%+v", d)
	}
}

func TestLyricsFromTranscript_ADroppedHalfIsAlsoRefused(t *testing.T) {
	reply := "STYLE: Ballad\nLYRICS:\n[Verse]\nCamino por la calle"
	a := setup(t, &fakeOllama{answer: reply})
	d, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: transcript})
	if err != nil || d.UnchangedWords {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestLyricsFromTranscript_ASongWithNoWordsAsksForTheStyleOnly(t *testing.T) {
	f := &fakeOllama{answer: "STYLE: Slow, calm and warm, with no vocals\nTAGS: calm,slow,warm"}
	a := setup(t, f)
	d, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Hint: "música para estudiar", BPM: 80, Engine: EngineHeartMuLa})
	if err != nil || d.Lyrics != "" || d.Caption == "" || d.Tags != "calm,slow" {
		t.Fatalf("%+v %v", d, err)
	}
	sys := f.chats[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "no words") || strings.Contains(sys, "LYRICS:") {
		t.Errorf("system = %q", sys)
	}
}

func TestLyricsFromTranscript_Limits(t *testing.T) {
	f := &fakeOllama{answer: "STYLE: x"}
	a := setup(t, f)
	if _, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: strings.Repeat("la ", MaxTranscriptLen)}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("a long transcription: %v", err)
	}
	if _, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: "hola", Engine: "suno"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("an unknown engine: %v", err)
	}
	if f.chatCount() != 0 {
		t.Fatal("the model must not be called for invalid input")
	}
}

func TestMusicSystemFor_TheLanguageRuleFollowsTheEngine(t *testing.T) {
	if s := musicSystemFor(EngineDiffRhythm2, false); !strings.Contains(s, "only reads English") || strings.Contains(s, "Spanish") {
		t.Errorf("DiffRhythm2 reads English only: %q", s)
	}
	if s := musicSystemFor(EngineHeartMuLa, false); !strings.Contains(s, "language of the user's description") || !strings.Contains(s, "even though STYLE and TAGS are in English") {
		t.Errorf("HeartMuLa keeps the language of the description: %q", s)
	}
}

func TestDropInventedInstruments(t *testing.T) {
	got := dropInventedInstruments("Pop ballad with melancholic mood, mid-tempo, Spanish vocals, acoustic guitar, soft piano", "")
	if got != "Pop ballad with melancholic mood, mid-tempo, Spanish vocals" {
		t.Errorf("got %q", got)
	}
	// an instrument the user named stays
	if got := dropInventedInstruments("slow, piano, guitar,sad", "una balada con piano"); got != "slow, piano,sad" {
		t.Errorf("got %q", got)
	}
	if got := dropInventedInstruments("folk,humorous,narrative voice", ""); got != "folk,humorous,narrative voice" {
		t.Errorf("nothing to drop: %q", got)
	}
}

func TestLyricsFromTranscript_DoesNotNameInstrumentsNobodyMentioned(t *testing.T) {
	reply := "STYLE: Slow ballad, melancholic, acoustic guitar, warm vocals\nTAGS: ballad,guitar,slow\nLYRICS:\n[Verse]\nCamino por la calle\nbajo la lluvia\nVuelvo a casa\nvuelvo a ti\nVuelvo a casa\nvuelvo a ti"
	a := setup(t, &fakeOllama{answer: reply})
	d, err := a.LyricsFromTranscript(context.Background(), TranscriptInput{Text: transcript, Engine: EngineHeartMuLa, Hint: "con piano"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.Caption, "guitar") || strings.Contains(d.Tags, "guitar") || d.Caption != "Slow ballad, melancholic, warm vocals" || d.Tags != "ballad,slow" {
		t.Fatalf("%q / %q", d.Caption, d.Tags)
	}
}
