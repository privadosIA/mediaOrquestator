package assist

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The music engines a track can use; they read different section tags and only the last two use tags.
const (
	EngineACEStep     = "ace-step"
	EngineHeartMuLa   = "heartmula"
	EngineDiffRhythm2 = "diffrhythm2"
)

// ParseEngine validates an engine name; empty means ACE-Step.
func ParseEngine(s string) (string, error) {
	switch s {
	case "", EngineACEStep:
		return EngineACEStep, nil
	case EngineHeartMuLa, EngineDiffRhythm2:
		return s, nil
	}
	return "", fmt.Errorf("%w: unknown generator %q (use %q, %q or %q)", ErrInvalidInput, s, EngineACEStep, EngineHeartMuLa, EngineDiffRhythm2)
}

// MusicOptions are what the music task needs besides the description.
type MusicOptions struct {
	Instrumental bool
	// Seconds is the length of the song, which sets the structure of the lyrics; 0 is a normal song.
	Seconds int
	// Engine is the track's generator (ParseEngine); empty is ACE-Step.
	Engine string
}

// engineSections are the structure tags HeartMuLa and DiffRhythm2 read (the same list; DiffRhythm2 also
// knows stop and space, which the assistant never writes).
var engineSections = map[string]bool{
	"intro": true, "verse": true, "chorus": true, "bridge": true, "outro": true,
	"inst": true, "solo": true, "hook": true, "break": true, "end": true,
}

var sectionLine = regexp.MustCompile(`^\s*\[([^\]]*)\]\s*$`)

// sectionWord is the word of a section tag line ("Verse 2", "Chorus:", "Pre-Chorus"): its first word in
// lower case without numbers or punctuation. For "Pre-Chorus" that is "pre-chorus", which no engine knows.
func sectionWord(inside string) string {
	s := strings.ToLower(strings.TrimSpace(inside))
	if i := strings.IndexAny(s, ":"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRightFunc(s, func(r rune) bool { return unicode.IsDigit(r) || unicode.IsSpace(r) || r == '.' })
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		s = s[:i]
	}
	return s
}

// EngineSections rewrites the section tags of lyrics for HeartMuLa and DiffRhythm2: a tag they know is
// written in lower case without numbers or notes (`[Verse 2]` becomes `[verse]`) and a tag they do not
// know is removed, so the engine never gets a line it would drop or read as a lyric.
func EngineSections(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		m := sectionLine.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		if w := sectionWord(m[1]); engineSections[w] {
			out = append(out, "["+w+"]")
		}
	}
	return out
}

// lyricsLanguageRule says in which language to write the lyrics. DiffRhythm2 only reads English (a Spanish
// lyric comes out badly pronounced), and for HeartMuLa the STYLE and TAGS lines being in English must not
// pull the lyrics into English too.
func lyricsLanguageRule(engine string) string {
	if engine == EngineDiffRhythm2 {
		return "Write the lyrics in English: this engine only reads English."
	}
	return "Write the lyrics in the language of the user's description (Spanish if it is in Spanish), even though STYLE and TAGS are in English."
}

// songShape is the order of the sections and the number of lines of a song of that length: the chorus comes
// back after every verse and the bridge only in a song that has room for it. 0 or no length is a normal song.
func songShape(seconds int) (sections []string, lines int) {
	switch {
	case seconds > 0 && seconds <= 75:
		return []string{"verse", "chorus", "verse", "chorus", "outro"}, 16
	case seconds > 150:
		return []string{"verse", "chorus", "verse", "chorus", "verse", "chorus", "bridge", "chorus", "outro"}, 36
	}
	return []string{"verse", "chorus", "verse", "chorus", "bridge", "chorus", "outro"}, 28
}

// lyricsShape tells the model how to write the lyrics for an engine: stanzas of four lines in the order of
// the song's length, the chorus repeated word for word, and the section tags in the case the engine reads.
func lyricsShape(engine string, seconds int) string {
	sections, lines := songShape(seconds)
	tags := make([]string, len(sections))
	for i, sec := range sections {
		if engine == EngineACEStep {
			sec = strings.ToUpper(sec[:1]) + sec[1:]
		}
		tags[i] = "[" + sec + "]"
	}
	return "the lyrics in stanzas of four lines (the outro may be two), with these section tags each on its own line, in this order: " +
		strings.Join(tags, ", ") + ". The chorus is the same four lines every time, word for word. About " + strconv.Itoa(lines) + " lines in total"
}

// musicSystemFor is the instruction of the music task for an engine and a normal-length song.
func musicSystemFor(engine string, instrumental bool) string {
	return musicSystemSized(engine, instrumental, 0)
}

// musicSystemSized is musicSystemFor for a song of the given length in seconds.
func musicSystemSized(engine string, instrumental bool, seconds int) string {
	if engine == EngineACEStep {
		if instrumental {
			return musicInstrumentalSystem
		}
		return "You help write songs for an AI music generator. Reply in exactly this format and nothing else:\n" +
			"STYLE: <ONE line in English, 20 to 40 words: genre, instruments, mood, tempo, vocals>\n" +
			"LYRICS:\n" +
			"<" + lyricsShape(engine, seconds) + ">\n" +
			"Write the lyrics in the same language as the user's description. " +
			"No preamble, no explanation, no code fences."
	}
	tags := "TAGS: <4 to 8 short English tags, a word or two each, in lower case, separated by commas with no space " +
		"after the commas: the genre, the main instruments, the mood, the tempo and the kind of voice>\n"
	if engine == EngineHeartMuLa {
		tags = "TAGS: <4 to 8 tags chosen ONLY from the list below, separated by commas with no space after the commas: " +
			"ONE genre first, then the mood, the main instruments, the tempo and the vocal style (at most one era)>\n" +
			"Allowed tags by category:\n" + tagVocabularyPrompt()
	}
	if instrumental {
		return "You help write songs for an AI music generator. Reply with exactly two lines and nothing else:\n" +
			"STYLE: <ONE line in English, 20 to 40 words: genre, instruments, mood, tempo; the song is instrumental, with no vocals>\n" +
			tags + "No preamble, no explanation, no lyrics."
	}
	return "You help write songs for an AI music generator. Reply in exactly this format and nothing else:\n" +
		"STYLE: <ONE line in English, 20 to 40 words: genre, instruments, mood, tempo, vocals>\n" +
		tags +
		"LYRICS:\n" +
		"<" + lyricsShape(engine, seconds) + ". Besides those, [intro], [inst] (an instrumental stretch), [solo], [hook], [break] and [end] are allowed when the song has such a part, and no other tag; every tag in lower case>\n" +
		lyricsLanguageRule(engine) +
		" No preamble, no explanation, no code fences."
}

// MaxTranscriptLen is the longest transcription the lyrics-from-transcript task accepts, in characters.
const MaxTranscriptLen = 8000

// TranscriptInput is what a song read from an audio or a video gives the assistant.
type TranscriptInput struct {
	// Text is the transcription, one segment per line; empty for a song with no words.
	Text     string
	Language string // ISO 639-1 code of the lyrics, when known
	Hint     string // what the user says about the song
	BPM      float64
	Key      string // "D minor"
	Picture  string // the description of a video's picture
	Engine   string
}

// SongDraft is what the lyrics-from-transcript task proposes.
type SongDraft struct {
	MusicSuggestion
	// UnchangedWords is true when the lyrics hold the words of the transcription and nothing else; false
	// when the model changed them and the transcription was kept as it was.
	UnchangedWords bool
}

const lyricsTaskRules = "Rules for the lyrics: use ONLY the lines of the transcription, in the order given, and never add, " +
	"complete, translate, correct or reword a single word. Put a section tag on its own line before each block. Choose the " +
	"sections by repetition: a block of lines that comes back is the chorus; the lines before it are verses; a different " +
	"block near the end is the bridge; a short block at the start can be the intro."

// LyricsFromTranscript orders the transcription of a song into sections and proposes its style and tags,
// without inventing text: the words the model returns are checked against the transcription.
func (a *Assistant) LyricsFromTranscript(ctx context.Context, in TranscriptInput) (SongDraft, error) {
	engine, err := ParseEngine(in.Engine)
	if err != nil {
		return SongDraft{}, err
	}
	text := strings.TrimSpace(in.Text)
	if len([]rune(text)) > MaxTranscriptLen {
		return SongDraft{}, fmt.Errorf("%w: the transcription is longer than %d characters", ErrInvalidInput, MaxTranscriptLen)
	}
	hasWords := text != ""
	system, max := lyricsFromTranscriptSystem(engine, hasWords), musicMaxTokens*2
	if !hasWords {
		max = musicInstrumentalMaxTokens
	}
	reply, truncated, err := a.generateOnCPU(ctx, system, transcriptUser(in, text), max)
	if err != nil {
		return SongDraft{}, err
	}
	if !hasStyleLine(reply) && IsRefusal(Clean(reply, false)) {
		return SongDraft{}, refused(a.Model)
	}
	sug, err := ParseMusicFor(reply, truncated, !hasWords, engine)
	if err != nil {
		return SongDraft{}, err
	}
	// Neither the lyrics nor the measures say which instruments play: only the user (or the picture) can.
	known := in.Hint + " " + in.Picture
	sug.Caption = dropInventedInstruments(sug.Caption, known)
	sug.Tags = dropInventedInstruments(sug.Tags, known)
	if sug.Caption == "" {
		return SongDraft{}, fmt.Errorf("%w: the model returned no style", ErrEmpty)
	}
	if !hasWords {
		return SongDraft{MusicSuggestion: sug, UnchangedWords: true}, nil
	}
	if sameWords(text, sug.Lyrics) && !truncated {
		return SongDraft{MusicSuggestion: sug, UnchangedWords: true}, nil
	}
	// The model rewrote the lyrics (or was cut short): keep what was heard, under one section tag.
	sug.Lyrics = firstSection(engine) + "\n" + text
	return SongDraft{MusicSuggestion: sug, UnchangedWords: false}, nil
}

func firstSection(engine string) string {
	if engine == EngineACEStep {
		return "[Verse]"
	}
	return "[verse]"
}

func lyricsFromTranscriptSystem(engine string, hasWords bool) string {
	sections := "[Verse], [Chorus], [Bridge] and [Outro]"
	if engine != EngineACEStep {
		sections = "in lower case [intro], [verse], [chorus], [bridge], [outro], [inst], [solo], [hook], [break] and [end]"
	}
	tags := ""
	if engine != EngineACEStep {
		tags = "TAGS: <4 to 8 short English tags, a word or two each, in lower case, separated by commas with no space after the commas: the genre, the mood, the tempo and the kind of voice>\n"
	}
	if !hasWords {
		return "You describe a song that has no words, from what is known about it. Reply with exactly these lines and nothing else:\n" +
			"STYLE: <ONE line in English, 15 to 30 words: genre, mood, tempo; name an instrument only if the user did, and no vocals>\n" +
			tags + "No preamble, no explanation."
	}
	return "You organise the transcription of a song. Reply in exactly this format and nothing else:\n" +
		"STYLE: <ONE line in English, 15 to 30 words: genre, mood, tempo and vocals, from the lyrics, the language and what the user says; name an instrument only if the user did>\n" +
		tags +
		"LYRICS:\n<the lyrics of the transcription with the section tags " + sections + ", each on its own line>\n" +
		lyricsTaskRules + " No preamble, no explanation, no code fences."
}

func transcriptUser(in TranscriptInput, text string) string {
	var b strings.Builder
	if in.Language != "" {
		fmt.Fprintf(&b, "Language of the lyrics: %s\n", in.Language)
	}
	if h := strings.TrimSpace(in.Hint); h != "" {
		fmt.Fprintf(&b, "What the user says about the song: %s\n", h)
	}
	if in.BPM > 0 {
		fmt.Fprintf(&b, "Measured tempo (an estimate): about %.0f bpm\n", in.BPM)
	}
	if in.Key != "" {
		fmt.Fprintf(&b, "Measured key (an estimate): %s\n", in.Key)
	}
	if p := strings.TrimSpace(in.Picture); p != "" {
		fmt.Fprintf(&b, "What the picture of its video shows: %s\n", p)
	}
	if text != "" {
		fmt.Fprintf(&b, "Transcription:\n%s\n", text)
	} else {
		b.WriteString("The song has no words.\n")
	}
	b.WriteString("(Style and tags in English.)")
	return b.String()
}

// sameWords reports whether the lyrics have the words of the transcription and nothing else: every word
// of the lyrics (section tags aside) is in the transcription, and at least 70 % of the transcription's
// words are in the lyrics. Case, accents' case and punctuation do not matter.
func sameWords(transcript, lyrics string) bool {
	have := wordsOf(transcript)
	if len(have) == 0 {
		return false
	}
	known := make(map[string]bool, len(have))
	for _, w := range have {
		known[w] = true
	}
	var body []string
	for _, l := range strings.Split(lyrics, "\n") {
		if sectionLine.MatchString(l) {
			continue
		}
		body = append(body, l)
	}
	got := wordsOf(strings.Join(body, "\n"))
	for _, w := range got {
		if !known[w] {
			return false
		}
	}
	return float64(len(got)) >= 0.7*float64(len(have))
}

func wordsOf(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' })
}

// instruments are the names the proposal must not invent.
var instruments = []string{
	"guitar", "piano", "drum", "bass", "violin", "viola", "cello", "string", "synth", "saxophone", "sax", "trumpet",
	"trombone", "horn", "brass", "flute", "clarinet", "oboe", "organ", "banjo", "ukulele", "harp", "accordion",
	"mandolin", "fiddle", "harmonica", "keyboard", "keys", "percussion", "orchestra", "bell", "marimba", "xylophone",
}

// dropInventedInstruments removes from a comma-separated list (a style or the tags) the items that name an
// instrument the user did not mention (in known, their hint and the picture's description). A model that is
// told not to name instruments still does when the lyrics suggest a genre; this does not leave it to the model.
func dropInventedInstruments(list, known string) string {
	known = strings.ToLower(known)
	parts := strings.Split(list, ",")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		low := strings.ToLower(p)
		invented := false
		for _, ins := range instruments {
			if strings.Contains(low, ins) && !strings.Contains(known, ins) {
				invented = true
				break
			}
		}
		if !invented {
			kept = append(kept, p)
		}
	}
	return strings.TrimSpace(strings.Join(kept, ","))
}
