package models

import (
	"encoding/json"
	"testing"
)

// A server that hides adult projects does not list the models made for adults; the one that shows everything does.
func TestCatalogAdult_ModelsMadeForAdultsAreListedOnlyWhenTheServerShowsEverything(t *testing.T) {
	saved := catalog
	t.Cleanup(func() { catalog = saved })
	catalog = append(append([]catalogEntry{}, catalog...), catalogEntry{ID: "adult-test", Kind: "model", Adult: true, Styles: []string{"x"}})
	for mode, want := range map[string]bool{"censored": false, "strict": false, "all": true} {
		s := newAssetServer(t)
		s.adult = want
		rec := call(s.Handler(), "GET", "/api/assets/catalog", "")
		var out struct {
			Entries []struct{ ID string }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		got := false
		for _, e := range out.Entries {
			got = got || e.ID == "adult-test"
		}
		if got != want {
			t.Errorf("mode %s: listed = %v, want %v", mode, got, want)
		}
	}
}
