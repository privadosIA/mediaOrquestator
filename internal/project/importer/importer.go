// Package importer converts external scene datasets — a JSON array of
// [id, title, lora_info, full_prompt] tuples, or an HTML page embedding
// that array in a `const data = [...]` script — into a scenes.yaml that
// passes the orchestrator's own validation (see openspec specs/scene-import).
//
// The HTML path is a thin extractor, not a JavaScript engine: it locates
// the array literal textually, resolves only ${NAME} interpolations of
// string constants declared in the same script, and parses the result as
// data. Anything beyond that rejects the affected entry, loudly.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Entry is one dataset tuple.
type Entry struct {
	ID         string
	Title      string
	LoraInfo   string
	FullPrompt string
}

// Rejection names an entry the loader or mapper refused, and why.
type Rejection struct {
	ID     string // may be empty when the id itself was unreadable
	Reason string
}

// Load reads a dataset from path: JSON when the first non-space byte is
// '[', the HTML extraction path otherwise.
func Load(path string) ([]Entry, []Rejection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("import: cannot read %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		entries, err := parseTuples([]byte(trimmed))
		if err != nil {
			return nil, nil, fmt.Errorf("import: %s: %w", path, err)
		}
		return entries, nil, nil
	}
	entries, rejected, err := loadHTML(string(data))
	if err != nil {
		return nil, nil, fmt.Errorf("import: %s: %w", path, err)
	}
	return entries, rejected, nil
}

// LoadBytes behaves exactly like Load for dataset content already in
// memory (an HTTP upload), with the same content-based JSON/HTML
// dispatch.
func LoadBytes(data []byte) ([]Entry, []Rejection, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		entries, err := parseTuples([]byte(trimmed))
		if err != nil {
			return nil, nil, fmt.Errorf("import: %w", err)
		}
		return entries, nil, nil
	}
	entries, rejected, err := loadHTML(string(data))
	if err != nil {
		return nil, nil, fmt.Errorf("import: %w", err)
	}
	return entries, rejected, nil
}

// parseTuples decodes a JSON array of [id, title, lora_info, full_prompt]
// string tuples.
func parseTuples(data []byte) ([]Entry, error) {
	var raw [][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("dataset is not an array of string tuples: %w", err)
	}
	entries := make([]Entry, 0, len(raw))
	for i, t := range raw {
		if len(t) != 4 {
			return nil, fmt.Errorf("entry %d has %d fields, want 4 ([id, title, lora_info, full_prompt])", i, len(t))
		}
		entries = append(entries, Entry{ID: t[0], Title: t[1], LoraInfo: t[2], FullPrompt: t[3]})
	}
	return entries, nil
}

var (
	constRe       = regexp.MustCompile(`(?m)^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`)
	placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// loadHTML extracts the `const data = [...]` array from the HTML's script,
// resolves ${NAME} interpolations of string constants declared in the same
// script, and parses the tuples. Entries with unresolved placeholders are
// rejected individually; the rest import normally.
func loadHTML(html string) ([]Entry, []Rejection, error) {
	scriptStart := strings.Index(html, "<script>")
	scriptEnd := strings.LastIndex(html, "</script>")
	if scriptStart == -1 || scriptEnd == -1 || scriptEnd < scriptStart {
		return nil, nil, fmt.Errorf("no <script> block found")
	}
	script := html[scriptStart+len("<script>") : scriptEnd]

	// String constants declared in the same script (e.g. const NEG = "…";).
	consts := map[string]string{}
	for _, m := range constRe.FindAllStringSubmatch(script, -1) {
		var val string
		if err := json.Unmarshal([]byte(`"`+m[2]+`"`), &val); err == nil {
			consts[m[1]] = val
		}
	}

	arrayLit, err := extractDataArray(script)
	if err != nil {
		return nil, nil, err
	}

	jsonLit, err := jsArrayToJSON(arrayLit)
	if err != nil {
		return nil, nil, err
	}
	entries, err := parseTuples([]byte(jsonLit))
	if err != nil {
		return nil, nil, err
	}

	// Resolve interpolations per entry; reject entries that keep ${…}.
	var accepted []Entry
	var rejected []Rejection
	for _, e := range entries {
		e.FullPrompt = placeholderRe.ReplaceAllStringFunc(e.FullPrompt, func(ph string) string {
			name := placeholderRe.FindStringSubmatch(ph)[1]
			if v, ok := consts[name]; ok {
				return v
			}
			return ph
		})
		if m := placeholderRe.FindStringSubmatch(e.FullPrompt); m != nil {
			rejected = append(rejected, Rejection{ID: e.ID, Reason: fmt.Sprintf("unresolved placeholder ${%s}", m[1])})
			continue
		}
		accepted = append(accepted, e)
	}
	return accepted, rejected, nil
}

// extractDataArray returns the balanced [...] literal following
// `const data =`, aware of single/double/backtick strings so brackets
// inside quotes do not count.
func extractDataArray(script string) (string, error) {
	idx := strings.Index(script, "const data")
	if idx == -1 {
		return "", fmt.Errorf("no `const data` declaration found")
	}
	start := strings.Index(script[idx:], "[")
	if start == -1 {
		return "", fmt.Errorf("`const data` is not followed by an array literal")
	}
	start += idx

	depth := 0
	var quote byte // 0 = not in a string; otherwise ', " or `
	for i := start; i < len(script); i++ {
		c := script[i]
		if quote != 0 {
			if c == '\\' {
				i++ // skip escaped char
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return script[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced `const data` array literal")
}

// jsArrayToJSON converts the extracted JS array literal to JSON: template
// literals and single-quoted strings become JSON strings, trailing commas
// are dropped. It is a character scanner, not a JS parser.
func jsArrayToJSON(lit string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(lit) {
		c := lit[i]
		switch c {
		case '`', '\'':
			end, content, err := scanString(lit, i, c)
			if err != nil {
				return "", err
			}
			enc, _ := json.Marshal(content)
			b.Write(enc)
			i = end
		case '"':
			end, content, err := scanString(lit, i, '"')
			if err != nil {
				return "", err
			}
			enc, _ := json.Marshal(content)
			b.Write(enc)
			i = end
		case ',':
			// Drop trailing commas (`, ]`): look ahead past whitespace.
			j := i + 1
			for j < len(lit) && (lit[j] == ' ' || lit[j] == '\n' || lit[j] == '\t' || lit[j] == '\r') {
				j++
			}
			if j < len(lit) && lit[j] == ']' {
				i++
				continue
			}
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}

// scanString consumes a quoted string starting at lit[start] (the opening
// quote) and returns the index just past the closing quote plus the
// decoded content.
func scanString(lit string, start int, quote byte) (int, string, error) {
	var content strings.Builder
	for i := start + 1; i < len(lit); i++ {
		c := lit[i]
		if c == '\\' && i+1 < len(lit) {
			next := lit[i+1]
			switch next {
			case 'n':
				content.WriteByte('\n')
			case 't':
				content.WriteByte('\t')
			case '\\', '\'', '"', '`', '$':
				content.WriteByte(next)
			default:
				content.WriteByte('\\')
				content.WriteByte(next)
			}
			i++
			continue
		}
		if c == quote {
			return i + 1, content.String(), nil
		}
		content.WriteByte(c)
	}
	return 0, "", fmt.Errorf("unterminated string literal")
}
