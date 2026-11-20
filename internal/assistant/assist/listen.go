package assist

import (
	"regexp"
	"strings"
)

// TaskListen describes a song that was heard (a recording of it playing) from what could be read of it: a few
// words of its lyrics when it has any, its tempo and its key. It is not offered through ParseTask: the listen
// endpoint is its only caller, and it goes through Run for the model's lifecycle.
const TaskListen Task = "listen"

// listenTask is the prompt of the task that describes a song someone recorded.
func listenTask() taskSpec {
	return taskSpec{
		system: "You describe a song that someone recorded with a phone, from what could be read of it. " +
			"You are given a few words of its lyrics (or none), its tempo and its key (estimates) and how it sounds (measured: " +
			"timbre, bass, texture, dynamics, density of the rhythm, loudness). " +
			"Reply with ONE line in exactly this format and nothing else:\n" +
			"STYLE=<genre, mood, main instruments and kind of voice, in English, 8 to 20 words, based only on what you are given>; " +
			"THEME=<what the lyrics are about, in 6 to 15 words, in the language of the lyrics; empty when there are no words>\n" +
			"The measured sound is real evidence of the genre; the words are evidence of the theme and the mood. " +
			"Never quote the lyrics and never name the song or the artist: describe the kind of song. " +
			"Do not default to lo-fi, ambient or a melancholic mood: say them only when the figures point to them. " +
			"When there are no words, say nothing about a voice (neither that there is one nor that there is none). " +
			"If you cannot tell the genre from what you are given, start STYLE with \"unknown genre,\" and describe only the sound. " +
			"No preamble, no quotes.",
		user:      func(text string) string { return text },
		maxTokens: 120,
	}
}

// ListenGuess is what the model understood of a recording.
type ListenGuess struct {
	// Style is the genre, mood, instruments and voice, in English.
	Style string
	// Theme is what the lyrics are about, in the language of the lyrics; empty without words.
	Theme string
}

// UnknownGenre is how the style of a recording starts when the model could not tell the genre.
const UnknownGenre = "unknown genre"

// GenreUnknown says whether the style is one the model could not put a genre to.
func (g ListenGuess) GenreUnknown() bool {
	return strings.HasPrefix(strings.ToLower(g.Style), UnknownGenre)
}

var (
	listenStyle = regexp.MustCompile(`(?i)\bSTYLE[\s*_]*[=:][\s*_]*([^;\n]*)`)
	listenTheme = regexp.MustCompile(`(?i)\bTHEME[\s*_]*[=:][\s*_]*([^;\n]*)`)
)

// ParseListenReply reads the one-line reply of TaskListen. It never fails: what it cannot read is left empty.
func ParseListenReply(reply string) ListenGuess {
	var g ListenGuess
	trim := func(s string) string { return strings.Trim(strings.TrimSpace(s), `"'«»“”. `) }
	if m := listenStyle.FindStringSubmatch(reply); m != nil {
		g.Style = trim(m[1])
	}
	if m := listenTheme.FindStringSubmatch(reply); m != nil {
		g.Theme = trim(m[1])
	}
	return g
}
