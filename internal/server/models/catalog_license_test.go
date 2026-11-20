package models

import "testing"

// Every entry says whether its license allows commercial use, and whatever is not a plain "yes" explains why.
func TestCatalogLicense_EveryEntryDeclaresCommercialUse(t *testing.T) {
	for _, e := range catalog {
		switch e.Commercial {
		case "yes":
		case "conditions", "no", "unknown":
			if e.CommercialNote == "" || e.CommercialNoteEN == "" {
				t.Errorf("%s: %q needs a note in both languages", e.ID, e.Commercial)
			}
		default:
			t.Errorf("%s: commercial is %q", e.ID, e.Commercial)
		}
		if e.License == "" {
			t.Errorf("%s: no license named", e.ID)
		}
	}
}

func TestCatalogLicense_KnownNonCommercialAndExcludedEntries(t *testing.T) {
	for id, want := range map[string]string{
		"image-edit-flux-kontext-q4":    "no",
		"image-edit-qwen-image-edit-q4": "yes",
		"music-diffrhythm2":             "no",
		"video-minimax-h3-fl2va-q4":     "conditions",
	} {
		if got := catalogEntryByID(t, id).Commercial; got != want {
			t.Errorf("%s: commercial %q, want %q", id, got, want)
		}
	}
}
