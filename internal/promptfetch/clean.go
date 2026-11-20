package promptfetch

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Limits and the words that make a prompt unfit for a shared library when its source has no score of its own.
const (
	MinLength = 12
	MaxLength = 2000
)

// Blocklist are explicit words: a prompt with one of them is dropped, for a source that has no NSFW score.
var Blocklist = []string{"nude", "naked", "nsfw", "porn", "sex", "sexual", "erotic", "hentai", "topless", "nipples", "genitals", "rape", "gore"}

// Rules are the reasons a row is dropped, in the order they are checked, for the report.
var Rules = []string{"empty", "nsfw", "too_short", "too_long", "url_only", "weights_only", "blocklist", "duplicate"}

// Counts says what happened to the rows of one source.
type Counts struct {
	Read    int            `json:"read"`
	Dropped map[string]int `json:"dropped"`
	Kept    int            `json:"kept"`
}

// Item is a preset a source gave, before its licence is added.
type Item struct {
	Kind     string
	Title    string
	Prompt   string
	Tags     []string
	Engine   string
	Language string
	Source   string
	SourceID string
	// NSFW is true only when the source judged it so; HasScore says the source has scores (so the field is written).
	HasScore bool
}

var (
	wsRe       = regexp.MustCompile(`\s+`)
	nonWordRe  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	weightRe   = regexp.MustCompile(`:\s*-?\d+(\.\d+)?`)
	bracketRe  = regexp.MustCompile(`[(){}\[\]]`)
	urlRe      = regexp.MustCompile(`(?i)^https?://\S+$`)
	wordsRe    = regexp.MustCompile(`\p{L}+`)
	camelRe    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	trailingRe = regexp.MustCompile(`[\s.:;!?-]+$`)
)

var spanish = map[string]bool{}

func init() {
	for _, w := range strings.Fields("el la los las un una de del que en y con por para es se su al lo como más pero sus le ya este sí porque esta entre cuando muy sin sobre") {
		spanish[w] = true
	}
}

func runeLen(s string) int { return len([]rune(s)) }

// Tidy collapses the whitespace of a prompt and trims it.
func Tidy(s string) string { return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")) }

// Normalise is what a prompt is compared by to find a duplicate: lower case, no punctuation, single spaces.
func Normalise(s string) string {
	return strings.TrimSpace(nonWordRe.ReplaceAllString(strings.ToLower(s), " "))
}

// LanguageOf says `es` when common Spanish words dominate the text, else the fallback: a label, not a detector.
func LanguageOf(text, fallback string) string {
	words := wordsRe.FindAllString(strings.ToLower(text), -1)
	if len(words) < 4 {
		return fallback
	}
	hits := 0
	for _, w := range words {
		if spanish[w] {
			hits++
		}
	}
	if hits >= 3 && float64(hits)/float64(len(words)) > 0.2 {
		return "es"
	}
	return fallback
}

// TitleOf is the subject: the text before the first comma (or the first 60 characters), without trailing punctuation.
func TitleOf(prompt string) string {
	first := strings.TrimSpace(strings.SplitN(prompt, ",", 2)[0])
	if r := []rune(first); len(r) > 60 {
		first = strings.TrimRightFunc(string(r[:57]), unicode.IsSpace) + "…"
	}
	if t := trailingRe.ReplaceAllString(first, ""); t != "" {
		return t
	}
	r := []rune(prompt)
	return string(r[:min(60, len(r))])
}

// TagsOf are the comma-separated fragments after the subject (lower-cased, 2 to 30 characters, no weights, no
// duplicates, at most 12) followed by the source's own tags, at most 16 in all.
func TagsOf(prompt string, own []string) []string {
	var fragments []string
	parts := strings.Split(prompt, ",")
	for _, f := range parts[min(1, len(parts)):] {
		f = bracketRe.ReplaceAllString(f, "")
		f = weightRe.ReplaceAllString(f, "")
		f = strings.ToLower(Tidy(f))
		if n := runeLen(f); n >= 2 && n <= 30 {
			fragments = append(fragments, f)
		}
	}
	if len(fragments) > 12 {
		fragments = fragments[:12]
	}
	for _, t := range own {
		if t = strings.ToLower(strings.TrimSpace(t)); runeLen(t) >= 2 && runeLen(t) <= 30 {
			fragments = append(fragments, t)
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range fragments {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

// Refusal says why a prompt is not kept, or "": too short, too long, only an address, only weights and numbers, or
// an explicit word.
func Refusal(prompt string, blocklist []string) string {
	switch {
	case runeLen(prompt) < MinLength:
		return "too_short"
	case runeLen(prompt) > MaxLength:
		return "too_long"
	case urlRe.MatchString(prompt):
		return "url_only"
	}
	hasLetter := false
	for _, r := range weightRe.ReplaceAllString(prompt, "") {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return "weights_only"
	}
	if len(blocklist) > 0 {
		words := map[string]bool{}
		for _, w := range wordsRe.FindAllString(strings.ToLower(prompt), -1) {
			words[w] = true
		}
		for _, w := range blocklist {
			if words[w] {
				return "blocklist"
			}
		}
	}
	return ""
}

// text turns a column value into text.
func text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprint(x)
	}
}

func camelWords(s string) string { return strings.ToLower(camelRe.ReplaceAllString(s, "$1 $2")) }

// ownTags are the tags a row carries from its own columns: a category, a list of style tags, a flag that is true.
func ownTags(row Row, s Source) []string {
	var out []string
	for _, t := range s.Columns.Tags {
		v := text(row[t.Column])
		if v == "" {
			continue
		}
		parts := []string{v}
		if t.Split != "" {
			parts = strings.Split(v, t.Split)
		}
		for _, p := range parts {
			if t.Camel {
				p = camelWords(p)
			}
			out = append(out, p)
		}
	}
	for _, f := range s.Columns.Flags {
		if strings.EqualFold(text(row[f.Column]), "true") {
			out = append(out, f.Tag)
		}
	}
	return out
}

func overNSFW(row Row, s Source) bool {
	for _, n := range s.NSFW {
		switch v := row[n.Column].(type) {
		case float64:
			if v > n.Max {
				return true
			}
		case int64:
			if float64(v) > n.Max {
				return true
			}
		}
	}
	return false
}

// Cleaner cleans the rows of one source as they stream by: it counts every row dropped under its rule and keeps the
// first of each duplicate.
type Cleaner struct {
	src    Source
	seen   map[string]bool
	Counts Counts
	Items  []Item
}

// NewCleaner makes the cleaner of a source.
func NewCleaner(s Source) *Cleaner {
	return &Cleaner{src: s, seen: map[string]bool{}, Counts: Counts{Dropped: map[string]int{}}}
}

// Add cleans one row.
func (c *Cleaner) Add(row Row) {
	s := c.src
	c.Counts.Read++
	drop := func(rule string) { c.Counts.Dropped[rule]++ }
	prompt := Tidy(text(row[s.Columns.Prompt]))
	if prompt == "" {
		drop("empty")
		return
	}
	if overNSFW(row, s) {
		drop("nsfw")
		return
	}
	// a source with a score of its own is judged by it; the block-list is for the ones without
	hasScore := len(s.NSFW) > 0
	blocklist := Blocklist
	if hasScore {
		blocklist = nil
	}
	if why := Refusal(prompt, blocklist); why != "" {
		drop(why)
		return
	}
	key := Normalise(prompt)
	if c.seen[key] {
		drop("duplicate")
		return
	}
	c.seen[key] = true
	id := strings.TrimSpace(text(row[s.Columns.SourceID]))
	if s.Columns.SourceID == "" || id == "" {
		sum := sha1.Sum([]byte(key))
		id = hex.EncodeToString(sum[:])[:12]
	}
	c.Items = append(c.Items, Item{Kind: s.Kind, Title: TitleOf(prompt), Prompt: prompt, Tags: TagsOf(prompt, ownTags(row, s)),
		Engine: s.Engine, Language: LanguageOf(prompt, orDefault(s.Language, "en")), Source: s.Repo, SourceID: id, HasScore: hasScore})
	c.Counts.Kept++
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
