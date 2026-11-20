package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// ownLookIDs are image models with a look of their own (declared and tested in the private edition's tests).
var ownLookIDs = map[string]bool{}

func TestCatalogStyles_DeclaredOnImageModelsOnly(t *testing.T) {
	want := map[string]struct {
		style string
		rank  int
	}{
		"krea2-intorealism-v4-fp8": {"photo", 1},
		"krea2-turbo-q4":           {"photo", 2},
		"sdxl-juggernaut-xl-v9":    {"photo", 3},
		"sdxl-pony-diffusion-v6":   {"anime", 1},
		"sdxl-base-1-0":            {"general", 1},
		"sd-1-5":                   {"general", 2},
	}
	for _, e := range catalog {
		w, ok := want[e.ID]
		if ownLookIDs[e.ID] {
			continue
		}
		if !ok {
			if len(e.Styles) != 0 || e.StyleRank != 0 {
				t.Errorf("%s: unexpected styles %v rank %d", e.ID, e.Styles, e.StyleRank)
			}
			continue
		}
		if len(e.Styles) != 1 || e.Styles[0] != w.style || e.StyleRank != w.rank {
			t.Errorf("%s: styles %v rank %d, want %s %d", e.ID, e.Styles, e.StyleRank, w.style, w.rank)
		}
	}
}

func TestCatalogQuality_OnlyTheWanBundles(t *testing.T) {
	want := map[string]string{
		"video-wan-2-2-ti2v-5b-q4":  "low",
		"video-wan-2-2-ti2v-5b-q8":  "medium",
		"video-wan-2-2-i2v-a14b-q4": "high",
	}
	for _, e := range catalog {
		if e.Quality != want[e.ID] {
			t.Errorf("%s: quality %q, want %q", e.ID, e.Quality, want[e.ID])
		}
	}
}

func TestCatalogStyles_OmittedWhenNotDeclared(t *testing.T) {
	b, _ := json.Marshal(catalogEntryByID(t, "voice-en-us-amy-medium"))
	for _, f := range []string{"styles", "style_rank", "quality"} {
		if strings.Contains(string(b), `"`+f+`":`) {
			t.Errorf("an entry without %s must not emit it: %s", f, b)
		}
	}
}
