package music

import (
	"regexp"
	"strings"
	"unicode"
)

// DefaultVocalLanguage is sung when a track has no language and its lyrics do
// not point to one.
const DefaultVocalLanguage = "en"

const (
	// minLanguageHits is how many hits the winning Latin-script language needs.
	minLanguageHits = 3
	// languageMargin is how much the winner must beat the runner-up by (x1.5,
	// written in tenths to stay in integers).
	languageMarginTenths = 15
)

// sectionTag matches the [Verse] / [Chorus] tags of a lyrics text.
var sectionTag = regexp.MustCompile(`\[[^\]\n]*\]`)

// latinWords holds very frequent words that belong to one language of the
// selector; words shared by several of them are left out on purpose.
var latinWords = map[string]string{}

func init() {
	for lang, words := range map[string]string{
		"es": "el los las del una unos unas es son soy eres estoy está están por para con sin pero porque cuando donde como más muy mi tu tus mis su sus nos yo ti te me se lo al y que en no sí amor corazón noche vida cielo",
		"en": "the and you your yours i'm i'll i've don't can't it's is are was were be been with that this for from my me we they love baby heart night oh yeah of to in on at not but",
		"fr": "le les des une est sont que qui pour dans pas je tu il elle nous vous ils mon ma mes ton ta tes avec sur sans mais comme très cœur amour nuit je suis c'est",
		"de": "der die das den dem des ein eine einen und ich du er sie es wir ihr ist sind nicht mit von zu auf für auch wie aber mein dein liebe herz nacht",
		"it": "il gli lo uno sono sei ho hai che non per con ma come più mio mia miei tuo tua tuoi amore cuore notte vita cielo ti ci è",
		"pt": "os um uma uns umas é são sou estou está não para com sem mas porque quando onde como mais muito meu minha teu tua seu sua nós eu você amor coração noite vida céu",
	} {
		for _, w := range strings.Fields(words) {
			if _, taken := latinWords[w]; taken {
				latinWords[w] = "" // shared by two languages: ignored
				continue
			}
			latinWords[w] = lang
		}
	}
}

// latinMarks are characters that give a Latin-script language away; each one
// counts as two word hits.
var latinMarks = map[rune]string{
	'ñ': "es", '¿': "es", '¡': "es",
	'ß': "de", 'ä': "de", 'ö': "de", 'ü': "de",
	'ã': "pt", 'õ': "pt",
	'ç': "fr", 'œ': "fr", 'ê': "fr", 'û': "fr",
}

// DetectLanguage guesses the vocal language of a lyrics text, as one of the
// codes the track form offers (es, en, fr, de, it, pt, ja, ko, zh, ru). It is
// local and deterministic: non-Latin scripts decide by rune counts, Latin
// script by frequent words and distinctive letters. Empty or unclear text
// gives DefaultVocalLanguage.
func DetectLanguage(lyrics string) string {
	text := strings.ToLower(sectionTag.ReplaceAllString(lyrics, " "))

	var hangul, kana, han, cyrillic int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		}
	}
	// Kana beats Han: Japanese mixes both, Chinese has no kana.
	switch {
	case hangul > 0 && hangul >= kana && hangul >= han && hangul >= cyrillic:
		return "ko"
	case kana > 0 && kana*2 >= han:
		return "ja"
	case han > 0 && han >= cyrillic:
		return "zh"
	case cyrillic > 0:
		return "ru"
	}

	hits := map[string]int{}
	for _, r := range text {
		if lang, ok := latinMarks[r]; ok {
			hits[lang] += 2
		}
	}
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\'' && r != '’'
	}) {
		w = strings.ReplaceAll(w, "’", "'")
		if lang := latinWords[w]; lang != "" {
			hits[lang]++
		}
	}

	best, second, bestLang := 0, 0, ""
	for _, lang := range []string{"es", "en", "fr", "de", "it", "pt"} { // fixed order: deterministic ties
		n := hits[lang]
		switch {
		case n > best:
			second, best, bestLang = best, n, lang
		case n > second:
			second = n
		}
	}
	if best >= minLanguageHits && best*10 >= second*languageMarginTenths {
		return bestLang
	}
	return DefaultVocalLanguage
}
