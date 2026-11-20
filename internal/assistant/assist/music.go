package assist

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// MusicSuggestion is what the music task writes: a style (caption, in
// English) and lyrics with section tags (in the language of the description,
// empty for an instrumental song). Raw is the model's reply as written.
type MusicSuggestion struct {
	Caption string
	Lyrics  string
	// Tags are the style words of the HeartMuLa and DiffRhythm2 engines, as the model wrote them (the caller
	// normalises them); empty for ACE-Step or when the model gave none.
	Tags string
	// NoGenre is true for HeartMuLa when no tag of the answer was a genre of the vocabulary.
	NoGenre bool
	Raw     string
}

const (
	musicSystem = "You help write songs for an AI music generator. Reply in exactly this format and nothing else:\n" +
		"STYLE: <ONE line in English, 20 to 40 words: genre, instruments, mood, tempo, vocals>\n" +
		"LYRICS:\n" +
		"<the lyrics, with the section tags [Verse], [Chorus], [Bridge] or [Outro] each on its own line; " +
		"two verses and a chorus, about 12 to 20 lines in total>\n" +
		"Write the lyrics in the same language as the user's description. " +
		"No preamble, no explanation, no code fences."

	musicInstrumentalSystem = "You help write songs for an AI music generator. Reply with exactly one line and nothing else:\n" +
		"STYLE: <ONE line in English, 20 to 40 words: genre, instruments, mood, tempo; the song is instrumental, with no vocals>\n" +
		"No preamble, no explanation, no lyrics."

	musicMaxTokens             = 1300
	musicInstrumentalMaxTokens = 120
)

// Music asks the model for the style and the lyrics of a song described in
// any language. With instrumental it asks for the style only. It shares the
// lifecycle of the other tasks: CPU only, memory check, unload when done.
func (a *Assistant) Music(ctx context.Context, text string, instrumental bool) (MusicSuggestion, error) {
	return a.MusicFor(ctx, text, MusicOptions{Instrumental: instrumental})
}

// MusicFor is Music for a given engine: for HeartMuLa and DiffRhythm2 it also asks for the tags and writes
// the section tags those engines read.
func (a *Assistant) MusicFor(ctx context.Context, text string, o MusicOptions) (MusicSuggestion, error) {
	if err := ValidateText(text); err != nil {
		return MusicSuggestion{}, err
	}
	engine, err := ParseEngine(o.Engine)
	if err != nil {
		return MusicSuggestion{}, err
	}
	system, max := musicSystemSized(engine, false, o.Seconds), musicMaxTokens
	user := strings.TrimSpace(text) + "\n(Style in English; lyrics in the language of the description above.)"
	if o.Instrumental {
		system, max = musicSystemSized(engine, true, o.Seconds), musicInstrumentalMaxTokens
		user = strings.TrimSpace(text) + "\n(Answer in English.)"
	}
	reply, truncated, err := a.generateOnCPU(ctx, system, user, max)
	if err != nil {
		return MusicSuggestion{}, err
	}
	if !hasStyleLine(reply) && IsRefusal(Clean(reply, false)) {
		return MusicSuggestion{}, refused(a.Model)
	}
	return ParseMusicFor(reply, truncated, o.Instrumental, engine)
}

var (
	styleLine  = regexp.MustCompile(`(?i)^[\s*#_>-]*style[\s*_]*:[\s*_]*(.*)$`)
	lyricsLine = regexp.MustCompile(`(?i)^[\s*#_>-]*lyrics[\s*_]*:[\s*_]*(.*)$`)
	tagsLine   = regexp.MustCompile(`(?i)^[\s*#_>-]*tags[\s*_]*:[\s*_]*(.*)$`)
)

// ParseMusic reads a model's reply in the STYLE:/LYRICS: format. Reasoning
// blocks, code fences and wrapping quotes are removed and the lyrics keep
// their line breaks. A reply without a STYLE marker uses its first line as the
// style. When the model stopped at its token limit the lyrics end at their last
// complete line. An empty style is an error, never an empty prompt.
func ParseMusic(reply string, truncated, instrumental bool) (MusicSuggestion, error) {
	return ParseMusicFor(reply, truncated, instrumental, EngineACEStep)
}

// ParseMusicFor is ParseMusic for an engine. For HeartMuLa and DiffRhythm2 it also reads the TAGS: line and
// keeps only the section tags those engines read, in lower case.
func ParseMusicFor(reply string, truncated, instrumental bool, engine string) (MusicSuggestion, error) {
	raw := strings.TrimSpace(reply)
	s := thinkRe.ReplaceAllString(reply, " ")
	if i := strings.Index(s, "<think>"); i >= 0 { // reasoning that never closed
		s = s[:i]
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") { // fences around the answer
			continue
		}
		lines = append(lines, strings.TrimRight(l, " \t"))
	}

	var style, tags, lyrics []string
	section := ""
	for _, l := range lines {
		if m := tagsLine.FindStringSubmatch(l); m != nil && section != "lyrics" {
			section = "tags"
			tags = append(tags, strings.TrimSpace(m[1]))
			continue
		}
		if m := styleLine.FindStringSubmatch(l); m != nil && section != "lyrics" {
			section = "style"
			style = append(style, strings.TrimSpace(m[1]))
			continue
		}
		if m := lyricsLine.FindStringSubmatch(l); m != nil {
			section = "lyrics"
			if rest := strings.TrimSpace(m[1]); rest != "" {
				lyrics = append(lyrics, rest)
			}
			continue
		}
		switch section {
		case "tags":
			if strings.TrimSpace(l) != "" {
				tags = append(tags, strings.TrimSpace(l))
			}
		case "style":
			if strings.TrimSpace(l) != "" {
				style = append(style, strings.TrimSpace(l))
			}
		case "lyrics":
			lyrics = append(lyrics, l)
		default:
			if len(style) == 0 && strings.TrimSpace(l) != "" {
				style = append(style, strings.TrimSpace(l)) // no STYLE marker: the first line is the style
				section = "style"
			}
		}
	}

	caption := trimWrappingQuotes(strings.Join(strings.Fields(strings.Join(style, " ")), " "))
	if caption == "" {
		return MusicSuggestion{}, fmt.Errorf("%w: the model returned no style", ErrEmpty)
	}
	tagText, noGenre := "", false
	if engine != EngineACEStep {
		tagText = strings.Trim(strings.TrimSpace(strings.Join(tags, ",")), ".,")
	}
	if engine == EngineHeartMuLa {
		var hasGenre bool
		tagText, hasGenre = NormalizeMusicTags(tagText)
		noGenre = !hasGenre
	}
	if instrumental {
		return MusicSuggestion{Caption: caption, Tags: tagText, NoGenre: noGenre, Raw: raw}, nil
	}
	if truncated && len(lyrics) > 0 {
		lyrics = lyrics[:len(lyrics)-1] // the last line was cut by the token limit
	}
	if engine != EngineACEStep {
		lyrics = EngineSections(lyrics)
	}
	return MusicSuggestion{Caption: caption, Lyrics: strings.TrimSpace(strings.Join(lyrics, "\n")), Tags: tagText, NoGenre: noGenre, Raw: raw}, nil
}

// hasStyleLine reports whether the reply has a "STYLE:" line.
func hasStyleLine(reply string) bool {
	for _, l := range strings.Split(strings.ReplaceAll(reply, "\r\n", "\n"), "\n") {
		if styleLine.MatchString(l) {
			return true
		}
	}
	return false
}
