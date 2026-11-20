package assist

import (
	"sort"
	"strings"
)

// TagCategory is a group of the tags HeartMuLa is asked for. HeartMuLa reads tags as free text, but it
// answers better to the kinds it was trained on, so the assistant chooses from this list and the server
// checks what it wrote. The list is a starting point taken from public descriptions of the model, not an
// official one: it is a table to edit.
type TagCategory struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`
}

// MusicTagCategories is the vocabulary, in the order the tags are written: the genre first, then the mood, the
// instruments and, to refine, the tempo, the vocal style, the era and the production (the order the public
// dictionary of the model recommends). The tags are written as that dictionary writes them.
var MusicTagCategories = []TagCategory{
	{ID: "genre", Tags: []string{"pop", "rock", "jazz", "electronic", "classical", "hip-hop", "r&b", "country", "reggae", "blues", "folk", "metal", "punk", "disco", "funk", "soul", "indie", "edm", "house", "techno", "ambient", "lo-fi", "k-pop", "j-pop", "c-pop", "latin", "reggaeton", "bossa nova"}},
	{ID: "mood", Tags: []string{"happy", "sad", "energetic", "calm", "romantic", "melancholic", "uplifting", "dark", "dreamy", "aggressive", "peaceful", "nostalgic", "hopeful", "mysterious", "playful", "epic", "chill", "intense"}},
	{ID: "instrument", Tags: []string{"piano", "guitar", "acoustic guitar", "electric guitar", "bass", "drums", "violin", "cello", "saxophone", "trumpet", "flute", "synth", "strings", "orchestra", "choir", "808", "harp", "organ", "ukulele", "harmonica"}},
	{ID: "tempo", Tags: []string{"slow", "moderate", "fast", "very fast", "ballad", "uptempo", "mid-tempo", "groovy"}},
	{ID: "vocal", Tags: []string{"male vocal", "female vocal", "duet", "rap", "whisper", "powerful vocals", "falsetto", "soprano", "baritone", "tenor", "alto", "harmonies", "a cappella"}},
	{ID: "era", Tags: []string{"80s", "90s", "2000s", "modern", "retro", "vintage", "70s", "60s", "futuristic"}},
	{ID: "production", Tags: []string{"cinematic", "hi-fi", "acoustic", "live", "studio", "raw", "polished", "minimalist", "layered", "atmospheric"}},
}

// TagCombo is a popular set of tags that makes the first choice easy.
type TagCombo struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// MusicTagCombos are the popular combinations of the public dictionary.
var MusicTagCombos = []TagCombo{
	{ID: "chill-lofi", Name: "Chill Lo-fi", Tags: []string{"lo-fi", "chill", "piano", "slow"}},
	{ID: "epic-cinematic", Name: "Epic Cinematic", Tags: []string{"cinematic", "epic", "orchestra", "strings"}},
	{ID: "upbeat-pop", Name: "Upbeat Pop", Tags: []string{"pop", "happy", "energetic", "female vocal"}},
	{ID: "dark-electronic", Name: "Dark Electronic", Tags: []string{"electronic", "dark", "synth", "modern"}},
	{ID: "acoustic-ballad", Name: "Acoustic Ballad", Tags: []string{"acoustic", "ballad", "guitar", "romantic"}},
	{ID: "retro-synthwave", Name: "Retro Synthwave", Tags: []string{"80s", "synth", "electronic", "retro"}},
	{ID: "jazz-cafe", Name: "Jazz Cafe", Tags: []string{"jazz", "piano", "saxophone", "calm"}},
	{ID: "hiphop-beat", Name: "Hip-Hop Beat", Tags: []string{"hip-hop", "808", "bass", "rap"}},
	{ID: "rock-anthem", Name: "Rock Anthem", Tags: []string{"rock", "electric guitar", "drums", "powerful vocals"}},
	{ID: "ambient-meditation", Name: "Ambient Meditation", Tags: []string{"ambient", "peaceful", "atmospheric", "slow"}},
}

// tagSynonyms maps what a model tends to write to the tag of the vocabulary.
var tagSynonyms = map[string]string{
	"hip hop": "hip-hop", "hiphop": "hip-hop", "trap": "hip-hop", "rnb": "r&b", "r and b": "r&b", "rhythm and blues": "r&b",
	"electro": "electronic", "electronica": "electronic", "techno music": "techno", "tech house": "house", "deep house": "house",
	"kpop": "k-pop", "jpop": "j-pop", "lofi": "lo-fi", "lo fi": "lo-fi", "synthwave": "electronic", "synth wave": "electronic",
	"synth-pop": "pop", "synthpop": "pop", "hard rock": "rock", "alternative": "indie", "film score": "cinematic",
	"male vocals": "male vocal", "male voice": "male vocal", "male singer": "male vocal", "male": "male vocal",
	"female vocals": "female vocal", "female voice": "female vocal", "female singer": "female vocal", "female": "female vocal",
	"rapping": "rap", "rap vocal": "rap", "rap vocals": "rap", "choral": "choir", "powerful": "powerful vocals", "acapella": "a cappella",
	"upbeat": "energetic", "lively": "energetic", "relaxed": "calm", "relaxing": "calm", "sorrowful": "sad", "gloomy": "dark",
	"dark mood": "dark", "intense mood": "intense", "joyful": "happy", "cheerful": "happy", "angry": "aggressive",
	"synthesizer": "synth", "synthesizers": "synth", "synths": "synth", "synth lead": "synth", "keyboard": "piano", "keys": "piano",
	"drum": "drums", "drum machine": "drums", "sax": "saxophone", "string": "strings", "bass guitar": "bass", "sub bass": "bass",
	"fast tempo": "fast", "up-tempo": "uptempo", "quick": "fast", "rapid": "fast", "super fast": "very fast", "super-fast": "very fast",
	"slow tempo": "slow", "medium tempo": "moderate", "medium": "moderate", "midtempo": "mid-tempo", "mid tempo": "mid-tempo",
	"orchestral": "orchestra", "cinematic score": "cinematic", "airy": "whisper", "breathy": "whisper",
	"eighties": "80s", "nineties": "90s", "sixties": "60s", "seventies": "70s", "futuristic sound": "futuristic",
}

// speedTags are the tempo words that say how fast; a suggestion keeps one of them.
var speedTags = map[string]bool{"slow": true, "moderate": true, "fast": true, "very fast": true, "mid-tempo": true, "uptempo": true}

// MaxMusicTags is the most tags a suggestion carries.
const MaxMusicTags = 8

// tagCategoryOf is the index in MusicTagCategories of the category of a tag.
var tagCategoryOf, _ = indexTags()

func indexTags() (category, order map[string]int) {
	category, order = map[string]int{}, map[string]int{}
	for i, c := range MusicTagCategories {
		for j, t := range c.Tags {
			category[t], order[t] = i, j
		}
	}
	return category, order
}

// NormalizeMusicTags checks the tags a model wrote against the vocabulary: lower case, synonyms mapped to
// their tag, tags outside the vocabulary and repeated ones dropped, at most two per category (one for the
// tempo), ordered by category and limited to MaxMusicTags. hasGenre says whether the
// result starts with a genre; when it does not the caller says the genre is missing.
func NormalizeMusicTags(text string) (tags string, hasGenre bool) {
	seen := map[string]bool{}
	speed := false // one tempo word of the speed kind: "fast" and "very fast" would contradict each other
	var kept []string
	perCategory := map[int]int{}
	for _, w := range strings.Split(text, ",") {
		w = strings.ToLower(strings.Join(strings.Fields(strings.Trim(w, " .\"'")), " "))
		if s, ok := tagSynonyms[w]; ok {
			w = s
		}
		cat, ok := tagCategoryOf[w]
		if !ok || seen[w] || perCategory[cat] >= maxPerCategory(cat) || (speedTags[w] && speed) {
			continue
		}
		if speedTags[w] {
			speed = true
		}
		seen[w] = true
		perCategory[cat]++
		kept = append(kept, w)
	}
	sort.SliceStable(kept, func(i, j int) bool { return tagCategoryOf[kept[i]] < tagCategoryOf[kept[j]] })
	if len(kept) > MaxMusicTags {
		kept = kept[:MaxMusicTags]
	}
	hasGenre = len(kept) > 0 && tagCategoryOf[kept[0]] == 0
	return strings.Join(kept, ","), hasGenre
}

// maxPerCategory is how many tags of a category a suggestion keeps: a few instruments, one era (two would
// contradict each other) and two of the rest ("slow" and "ballad" go together).
func maxPerCategory(cat int) int {
	switch MusicTagCategories[cat].ID {
	case "era":
		return 1
	case "instrument":
		return 3
	}
	return 2
}

// tagVocabularyPrompt lists the vocabulary and the popular combinations for the instruction of the music task.
func tagVocabularyPrompt() string {
	var b strings.Builder
	for _, c := range MusicTagCategories {
		b.WriteString(c.ID + ": " + strings.Join(c.Tags, ", ") + "\n")
	}
	b.WriteString("Examples of good sets of tags:\n")
	for _, c := range MusicTagCombos {
		b.WriteString("- " + strings.Join(c.Tags, ",") + "\n")
	}
	return b.String()
}
