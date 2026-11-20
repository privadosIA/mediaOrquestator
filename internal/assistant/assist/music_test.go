package assist

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodReply = "STYLE: Slow Spanish pop ballad with piano and warm male vocals, melancholic, 70 bpm\nLYRICS:\n[Verse]\nCamino por la calle bajo la lluvia\nLa ciudad duerme y yo sigo despierto\n\n[Chorus]\nVuelvo a casa, vuelvo a ti"

func TestMusic_AsksForAnEnglishStyleAndLyricsInTheLanguageOfTheDescription(t *testing.T) {
	f := &fakeOllama{answer: goodReply}
	a := setup(t, f)
	got, err := a.Music(context.Background(), "Una balada sobre volver a casa bajo la lluvia", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Caption, "Slow Spanish pop ballad") || strings.Contains(got.Caption, "\n") {
		t.Errorf("caption = %q", got.Caption)
	}
	if !strings.HasPrefix(got.Lyrics, "[Verse]\nCamino por la calle") || !strings.Contains(got.Lyrics, "\n\n[Chorus]") {
		t.Errorf("lyrics must keep their line breaks: %q", got.Lyrics)
	}
	req := f.chats[0]
	opts := req["options"].(map[string]any)
	if opts["num_gpu"] != float64(0) || opts["num_predict"] != float64(1300) {
		t.Errorf("options = %v", opts)
	}
	msgs := req["messages"].([]any)
	sys := msgs[0].(map[string]any)["content"].(string)
	usr := msgs[1].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "STYLE:") || !strings.Contains(sys, "LYRICS:") || !strings.Contains(sys, "same language as the user's description") {
		t.Errorf("system prompt = %q", sys)
	}
	if !strings.Contains(usr, "volver a casa bajo la lluvia") {
		t.Errorf("user prompt = %q", usr)
	}
	if f.unloadCount() != 1 {
		t.Errorf("the model must be unloaded when done, unloads = %d", f.unloadCount())
	}
}

func TestMusic_InstrumentalAsksForTheStyleOnly(t *testing.T) {
	f := &fakeOllama{answer: "STYLE: Relaxed lo-fi hip hop, mellow piano, soft drums, instrumental, 80 bpm"}
	a := setup(t, f)
	got, err := a.Music(context.Background(), "música tranquila para estudiar", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lyrics != "" || !strings.HasPrefix(got.Caption, "Relaxed lo-fi") {
		t.Errorf("%+v", got)
	}
	req := f.chats[0]
	if req["options"].(map[string]any)["num_predict"] != float64(120) {
		t.Errorf("instrumental needs a small limit: %v", req["options"])
	}
	sys := req["messages"].([]any)[0].(map[string]any)["content"].(string)
	if strings.Contains(sys, "LYRICS:") || !strings.Contains(sys, "no vocals") {
		t.Errorf("system prompt = %q", sys)
	}
}

func TestMusic_InstrumentalIgnoresLyricsTheModelWroteAnyway(t *testing.T) {
	f := &fakeOllama{answer: goodReply}
	a := setup(t, f)
	got, err := a.Music(context.Background(), "una balada", true)
	if err != nil || got.Lyrics != "" || got.Caption == "" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestMusic_SharesTheSafetyRulesOfTheOtherTasks(t *testing.T) {
	// refuses empty and over-long descriptions before calling the model
	f := &fakeOllama{answer: goodReply}
	a := setup(t, f)
	if _, err := a.Music(context.Background(), "  ", false); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty text: %v", err)
	}
	if _, err := a.Music(context.Background(), strings.Repeat("x", MaxTextLen+1), false); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("long text: %v", err)
	}
	if f.chatCount() != 0 {
		t.Fatal("the model must not be called for invalid input")
	}
	// refuses to run when memory is short
	b := setup(t, &fakeOllama{answer: goodReply})
	b.MemAvailable = func() (int64, error) { return 1 << 30, nil }
	if _, err := b.Music(context.Background(), "una balada", false); !errors.Is(err, ErrNoMemory) {
		t.Errorf("short memory: %v", err)
	}
	// and the model must not have landed in VRAM
	c := setup(t, &fakeOllama{answer: goodReply, vram: 1 << 30})
	if _, err := c.Music(context.Background(), "una balada", false); !errors.Is(err, ErrOnGPU) {
		t.Errorf("model on a GPU: %v", err)
	}
}

func TestMusic_EmptyStyleIsAnErrorNotAnEmptyPrompt(t *testing.T) {
	a := setup(t, &fakeOllama{answer: "<think>hmm</think>   "})
	if _, err := a.Music(context.Background(), "una balada", false); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseMusic(t *testing.T) {
	cases := []struct {
		name         string
		reply        string
		truncated    bool
		instrumental bool
		caption      string
		lyrics       string
		wantErr      bool
	}{
		{"plain", goodReply, false, false, "Slow Spanish pop ballad with piano and warm male vocals, melancholic, 70 bpm",
			"[Verse]\nCamino por la calle bajo la lluvia\nLa ciudad duerme y yo sigo despierto\n\n[Chorus]\nVuelvo a casa, vuelvo a ti", false},
		{"reasoning removed", "<think>plan the song</think>\nSTYLE: Folk\nLYRICS:\n[Verse]\nla", false, false, "Folk", "[Verse]\nla", false},
		{"reasoning never closed", "STYLE: Folk\nLYRICS:\n[Verse]\nla\n<think>and then", false, false, "Folk", "[Verse]\nla", false},
		{"code fences", "```\nSTYLE: Folk\nLYRICS:\n[Verse]\nla\n```", false, false, "Folk", "[Verse]\nla", false},
		{"markdown bold markers", "**STYLE:** Folk guitar\n**LYRICS:**\n[Verse]\nla", false, false, "Folk guitar", "[Verse]\nla", false},
		{"lowercase markers", "style: Folk\nlyrics:\n[Verse]\nla", false, false, "Folk", "[Verse]\nla", false},
		{"lyrics start on the marker line", "STYLE: Folk\nLYRICS: [Verse]\nla", false, false, "Folk", "[Verse]\nla", false},
		{"style split over lines", "STYLE: Folk guitar,\nwarm vocals\nLYRICS:\n[Verse]\nla", false, false, "Folk guitar, warm vocals", "[Verse]\nla", false},
		{"wrapping quotes", "STYLE: \"Folk guitar\"\nLYRICS:\n[Verse]\nla", false, false, "Folk guitar", "[Verse]\nla", false},
		{"no style marker: first line is the style", "A warm folk song\nLYRICS:\n[Verse]\nla", false, false, "A warm folk song", "[Verse]\nla", false},
		{"style only", "STYLE: Folk", false, false, "Folk", "", false},
		{"truncated: last cut line is dropped", "STYLE: Folk\nLYRICS:\n[Verse]\nCamino por la\nLa ciudad du", true, false, "Folk", "[Verse]\nCamino por la", false},
		{"not truncated keeps every line", "STYLE: Folk\nLYRICS:\n[Verse]\nCamino por la\nLa ciudad du", false, false, "Folk", "[Verse]\nCamino por la\nLa ciudad du", false},
		{"instrumental drops lyrics", goodReply, false, true, "Slow Spanish pop ballad with piano and warm male vocals, melancholic, 70 bpm", "", false},
		{"windows line endings", "STYLE: Folk\r\nLYRICS:\r\n[Verse]\r\nla", false, false, "Folk", "[Verse]\nla", false},
		{"empty", "", false, false, "", "", true},
		{"only reasoning", "<think>x</think>", false, false, "", "", true},
	}
	for _, c := range cases {
		got, err := ParseMusic(c.reply, c.truncated, c.instrumental)
		if c.wantErr {
			if !errors.Is(err, ErrEmpty) {
				t.Errorf("%s: err = %v, want ErrEmpty", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.Caption != c.caption || got.Lyrics != c.lyrics {
			t.Errorf("%s:\n caption %q (want %q)\n lyrics  %q (want %q)", c.name, got.Caption, c.caption, got.Lyrics, c.lyrics)
		}
	}
}

func TestParseTaskAcceptsMusicAndRunRefusesIt(t *testing.T) {
	if task, err := ParseTask("music"); err != nil || task != TaskMusic {
		t.Fatalf("%v %v", task, err)
	}
	if _, err := ParseTask("song"); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "music") {
		t.Fatalf("err = %v", err)
	}
	a := setup(t, &fakeOllama{answer: "x"})
	if _, err := a.Run(context.Background(), TaskMusic, "una balada"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Run must not handle the two-part task: %v", err)
	}
}

func TestNormalizeMusicTags_KeepsTheVocabularyAndDropsTheRest(t *testing.T) {
	got, genre := NormalizeMusicTags("Fast, Techno, rap, super-fast, chickenpluckercore, Male Vocals, techno, energetic, Hip Hop, synthesizer")
	if want := "techno,hip-hop,energetic,synth,fast,rap,male vocal"; got != want {
		t.Fatalf("tags = %q, want %q", got, want)
	}
	if !genre {
		t.Fatal("a genre was written")
	}
}

func TestNormalizeMusicTags_UsesTheSpellingOfTheDictionary(t *testing.T) {
	got, _ := NormalizeMusicTags("hiphop, lofi, rnb, kpop, medium tempo, dark mood")
	if want := "hip-hop,lo-fi,dark,moderate"; got != want { // two genres at most, the rest is a mood and a tempo
		t.Fatalf("tags = %q, want %q", got, want)
	}
}

func TestNormalizeMusicTags_NoGenreIsReported(t *testing.T) {
	got, genre := NormalizeMusicTags("happy, piano, nonsense")
	if genre || got != "happy,piano" {
		t.Fatalf("got %q genre=%v", got, genre)
	}
	if got, genre := NormalizeMusicTags("nonsense"); got != "" || genre {
		t.Fatalf("got %q genre=%v", got, genre)
	}
}

func TestNormalizeMusicTags_LimitsAreEnforced(t *testing.T) {
	got, _ := NormalizeMusicTags("pop,rock,jazz,happy,sad,calm,piano,bass,drums,strings,80s,90s")
	if n := len(strings.Split(got, ",")); n > MaxMusicTags {
		t.Fatalf("%d tags: %q", n, got)
	}
	if strings.Contains(got, "jazz") || strings.Contains(got, "90s") { // at most two genres and one era
		t.Fatalf("a tag over the limit was kept: %q", got)
	}
}

func TestEveryComboIsMadeOfVocabularyTags(t *testing.T) {
	if len(MusicTagCombos) != 10 {
		t.Fatalf("%d combinations", len(MusicTagCombos))
	}
	for _, c := range MusicTagCombos {
		for _, tag := range c.Tags {
			if _, ok := tagCategoryOf[tag]; !ok {
				t.Errorf("%s: %q is not in the vocabulary", c.ID, tag)
			}
		}
	}
}

func TestLyricsShape_FollowsTheLengthOfTheSong(t *testing.T) {
	short := lyricsShape(EngineHeartMuLa, 60)
	if !strings.Contains(short, "[verse], [chorus], [verse], [chorus], [outro]") || strings.Contains(short, "[bridge]") || !strings.Contains(short, "About 16 lines") {
		t.Errorf("short: %s", short)
	}
	normal := lyricsShape(EngineHeartMuLa, 0)
	if !strings.Contains(normal, "[bridge], [chorus], [outro]") || !strings.Contains(normal, "About 28 lines") {
		t.Errorf("normal: %s", normal)
	}
	long := lyricsShape(EngineACEStep, 180)
	if !strings.Contains(long, "[Verse], [Chorus], [Verse], [Chorus], [Verse], [Chorus], [Bridge], [Chorus], [Outro]") || !strings.Contains(long, "About 36 lines") {
		t.Errorf("long: %s", long)
	}
	if !strings.Contains(long, "word for word") {
		t.Error("the chorus is repeated verbatim")
	}
}

func TestParseMusicFor_HeartMuLaChecksItsTagsAndOthersDoNot(t *testing.T) {
	reply := "STYLE: fast techno rap\nTAGS: Techno, rap, invented-tag, fast\nLYRICS:\n[verse]\nhola\n"
	hm, err := ParseMusicFor(reply, false, false, EngineHeartMuLa)
	if err != nil || hm.Tags != "techno,fast,rap" || hm.NoGenre {
		t.Fatalf("heartmula: %+v %v", hm, err)
	}
	dr, err := ParseMusicFor(reply, false, false, EngineDiffRhythm2)
	if err != nil || !strings.Contains(dr.Tags, "invented-tag") {
		t.Fatalf("diffrhythm2 keeps its tags: %+v %v", dr, err)
	}
	if got := musicSystemFor(EngineHeartMuLa, false); !strings.Contains(got, "Allowed tags by category") || !strings.Contains(got, "techno") || !strings.Contains(got, "Examples of good sets of tags") {
		t.Fatalf("the prompt lists the vocabulary: %s", got)
	}
	if got := musicSystemFor(EngineDiffRhythm2, false); strings.Contains(got, "Allowed tags by category") {
		t.Fatal("DiffRhythm2 has no vocabulary")
	}
}
