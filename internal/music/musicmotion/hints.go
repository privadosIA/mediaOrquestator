package musicmotion

// hints.go: what is added to a refusal when the assistant is asked again, so that the message says how to fix it. The
// validators keep their messages (the API returns them to people); the help is only for the assistant.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// designHint explains a refused accent with the numbers the assistant needs: the contrast with both tones. It is
// empty for any other refusal, or when the accent is not a colour.
func designHint(d Design, err error) string {
	var contrast *AccentContrastError
	if !errors.As(err, &contrast) || !hexColour.MatchString(d.Accent) {
		return ""
	}
	if !hexColour.MatchString(d.Dark) || !hexColour.MatchString(d.Light) {
		return ""
	}
	return fmt.Sprintf("\nThe accent %s has a contrast of %.1f:1 with \"dark\" (%s) and %.1f:1 with \"light\" (%s); it needs %.0f:1 against the background of every mode in use. "+
		"Choose another accent that passes against the tones of the modes you use, or use only one mode for every section.",
		d.Accent, Contrast(d.Accent, d.Dark), d.Dark, Contrast(d.Accent, d.Light), d.Light, MinAccentContrast)
}

var reRefusedParam = regexp.MustCompile(`the parameter "([^"]+)" of the part "([^"]+)"`)

// scriptHint adds, to a refused parameter, the limits of that parameter of that part (read from the catalog) and, to a
// keyword or a hit word that is not in its line, the rule to copy it. It is empty for any other refusal.
func scriptHint(cat Catalog, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	var hint string
	if m := reRefusedParam.FindStringSubmatch(msg); m != nil {
		if part, ok := cat.Get(m[2]); ok {
			for _, p := range part.Params {
				if p.Name == m[1] {
					hint += fmt.Sprintf("\nThe limits of the parameter %q of the part %q: %s.", p.Name, part.Name, paramLimits(p))
				}
			}
		}
	}
	if strings.Contains(msg, "is not a word of") {
		hint += "\nCopy the keyword (or the word of the hit) exactly as it is written in the sung line, one word, without changing its form."
	}
	return hint
}

// paramLimits says in words what a parameter accepts.
func paramLimits(p Param) string {
	switch p.Type {
	case "string":
		if p.MaxLength > 0 {
			return fmt.Sprintf("text of at most %d characters", p.MaxLength)
		}
		return "text"
	case "enum":
		return "exactly one of " + strings.Join(p.Options, ", ")
	case "bool":
		return "true or false"
	case "int", "number":
		kind := "a number"
		if p.Type == "int" {
			kind = "a whole number"
		}
		switch {
		case p.Min != nil && p.Max != nil:
			return fmt.Sprintf("%s between %v and %v", kind, *p.Min, *p.Max)
		case p.Min != nil:
			return fmt.Sprintf("%s of at least %v", kind, *p.Min)
		case p.Max != nil:
			return fmt.Sprintf("%s of at most %v", kind, *p.Max)
		}
		return kind
	}
	return p.Type
}
