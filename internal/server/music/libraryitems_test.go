package music

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const libSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="var(--accent)"/></svg>`

func catBody(name, svg string) string {
	manifest := `{"name":"` + name + `","description":"A cat","version":1,"width":10,"height":10,"anchor":{"x":0.5,"y":1},"poses":{"idle":"idle.svg"},"expressions":["happy"],"default_pose":"idle","default_expression":"happy"}`
	b, _ := json.Marshal(map[string]any{"manifest": json.RawMessage(manifest), "files": map[string]string{"idle.svg": svg}})
	return string(b)
}

func TestLibraryCharacters_AreAddedChangedAndDeletedThroughTheAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, _ := newProjectsServer(t)
	const base = "/api/music-motion/library/characters"

	// the list: the ones of the program, each with its source
	if l := idsOfNames(t, s, base, "characters"); len(l.names) != 3 || l.names[0] != "bot" {
		t.Fatalf("%+v", l)
	}
	// a drawing with a script is refused and nothing is saved
	rec := call(s.Handler(), http.MethodPost, base, catBody("cat", `<svg><script>alert(1)</script></svg>`))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "script") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := call(s.Handler(), http.MethodGet, base+"/cat", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("nothing was saved: %d", rec.Code)
	}
	// a character is added, and the next catalog has it
	rec = call(s.Handler(), http.MethodPost, base, catBody("cat", libSVG))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"source":"user"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if l := idsOfNames(t, s, "/api/music-motion/catalog", "characters"); !contains(l.names, "cat") {
		t.Fatalf("the catalog lists it: %+v", l)
	}
	// a name that exists is a conflict, one of the program too
	for _, name := range []string{"cat", "bot"} {
		if rec := call(s.Handler(), http.MethodPost, base, catBody(name, libSVG)); rec.Code != http.StatusConflict {
			t.Fatalf("%s: %d", name, rec.Code)
		}
	}
	// replace one of the user's; the manifest must keep its name
	newer := strings.Replace(libSVG, "var(--accent)", "var(--fg)", 1)
	if rec := call(s.Handler(), http.MethodPut, base+"/cat", catBody("cat", newer)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "var(--fg)") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := call(s.Handler(), http.MethodPut, base+"/cat", catBody("dog", newer)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("another name: %d", rec.Code)
	}
	if rec := call(s.Handler(), http.MethodPut, base+"/ghost", catBody("ghost", newer)); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", rec.Code)
	}
	// the one of the program stays
	for _, m := range []string{http.MethodPut, http.MethodDelete} {
		body := ""
		if m == http.MethodPut {
			body = catBody("bot", libSVG)
		}
		if rec := call(s.Handler(), m, base+"/bot", body); rec.Code != http.StatusForbidden {
			t.Fatalf("%s of the program: %d", m, rec.Code)
		}
	}
	if rec := call(s.Handler(), http.MethodGet, base+"/bot", ""); rec.Code != http.StatusOK {
		t.Fatalf("%d", rec.Code)
	}
	// paging and search
	if l := idsOfNames(t, s, base+"?q=CAT&limit=5", "characters"); len(l.names) != 1 || l.names[0] != "cat" || l.total != "1" {
		t.Fatalf("search: %+v", l)
	}
	if l := idsOfNames(t, s, base+"?sort=-name&limit=2&offset=1", "characters"); len(l.names) != 2 || l.names[0] != "mascot" || l.names[1] != "cat" || l.total != "4" {
		t.Fatalf("a page of the reverse order: %+v", l)
	}
	if l := idsOfNames(t, s, base+"?sort=colour", "characters"); l.code != http.StatusBadRequest {
		t.Fatalf("%d", l.code)
	}
	// delete one of the user's
	if rec := call(s.Handler(), http.MethodDelete, base+"/cat", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call(s.Handler(), http.MethodGet, base+"/cat", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("gone from the read: %d", rec.Code)
	}
	if l := idsOfNames(t, s, "/api/music-motion/catalog", "characters"); contains(l.names, "cat") {
		t.Fatalf("gone from the catalog: %+v", l)
	}
	if rec := call(s.Handler(), http.MethodDelete, base+"/cat", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}

func TestLibraryBackgrounds_AreManagedLikeCharacters(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, _ := newProjectsServer(t)
	const base = "/api/music-motion/library/backgrounds"
	body := func(name, svg string) string {
		b, _ := json.Marshal(map[string]any{"manifest": map[string]any{"name": name, "description": "dusk", "version": 1, "file": "bg.svg"}, "files": map[string]string{"bg.svg": svg}})
		return string(b)
	}
	if rec := call(s.Handler(), http.MethodPost, base, body("dusk", libSVG)); rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"source":"user"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := call(s.Handler(), http.MethodPost, base, body("dusk", libSVG)); rec.Code != http.StatusConflict {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call(s.Handler(), http.MethodPost, base, body("loud", `<svg onload="x()"></svg>`)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call(s.Handler(), http.MethodDelete, base+"/night-sky", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("%d", rec.Code)
	}
	if l := idsOfNames(t, s, base+"?q=dusk", "backgrounds"); len(l.names) != 1 {
		t.Fatalf("%+v", l)
	}
	if rec := call(s.Handler(), http.MethodDelete, base+"/dusk", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call(s.Handler(), http.MethodGet, base+"/dusk", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}

type namesList struct {
	code  int
	names []string
	total string
}

func idsOfNames(t *testing.T, s *testServer, path, key string) namesList {
	t.Helper()
	rec := call(s.Handler(), http.MethodGet, path, "")
	out := namesList{code: rec.Code, total: rec.Header().Get("X-Total-Count")}
	if rec.Code == http.StatusOK {
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		var items []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(body[key], &items); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, it := range items {
			out.names = append(out.names, it.Name)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
