package promptfetch

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AllowedLicences are the licences a source may declare: those that allow redistributing the text with its attribution.
// Anything else, `unknown` and an absent licence included, refuses the source.
var AllowedLicences = []string{"cc0-1.0", "cc-by-4.0", "apache-2.0", "mit"}

// Card is what the command reads of a dataset's card on the Hugging Face API.
type Card struct {
	SHA      string          `json:"sha"`
	License  json.RawMessage `json:"license"`
	CardData struct {
		License json.RawMessage `json:"license"`
	} `json:"cardData"`
}

// Licences are the licences a card declares (a string or a list), lower-cased; `unknown` when it says nothing.
func (c Card) Licences() []string {
	raw := c.CardData.License
	if len(raw) == 0 || string(raw) == "null" {
		raw = c.License
	}
	var list []string
	var one string
	var many []string
	switch {
	case json.Unmarshal(raw, &one) == nil && one != "":
		list = []string{one}
	case json.Unmarshal(raw, &many) == nil:
		list = many
	}
	var out []string
	for _, l := range list {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return []string{"unknown"}
	}
	return out
}

func allowed(l string) bool {
	for _, a := range AllowedLicences {
		if a == l {
			return true
		}
	}
	return false
}

// Gate says whether a source may be used: every licence the card declares is in the allow-list and the source's
// expected licence is among them. When it may not, reason says why.
func Gate(c Card, s Source) (licences []string, ok bool, reason string) {
	licences = c.Licences()
	for _, l := range licences {
		if !allowed(l) {
			return licences, false, fmt.Sprintf("licence %s is not in the allow-list (%s)", strings.Join(licences, ", "), strings.Join(AllowedLicences, ", "))
		}
	}
	for _, l := range licences {
		if l == s.ExpectedLicence {
			return licences, true, ""
		}
	}
	return licences, false, fmt.Sprintf("the card says %s but the sources file expects %s", strings.Join(licences, ", "), s.ExpectedLicence)
}
