package generation

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGenerations_ArePagedAndKeptToAClient(t *testing.T) {
	s := genServer(t, fullCatalog())
	var ids []string
	for i, client := range []string{"alpha", "beta", "alpha"} {
		if i > 0 {
			time.Sleep(1100 * time.Millisecond) // the creation times have a resolution of a second
		}
		g := decodeGeneration(t, postGeneration(t, s, map[string]any{"prompt": "p", "option": "ltx", "client": client}))
		waitGeneration(t, s, g.ID, GenDone)
		ids = append(ids, g.ID)
	}
	list := func(query string) ([]generationView, http.Header, int) {
		rec := doJSON(t, s.Handler(), http.MethodGet, "/api/generations"+query, nil)
		var out []generationView
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out, rec.Header(), rec.Code
	}
	all, hdr, _ := list("")
	if len(all) != 3 || all[0].ID != ids[2] || hdr.Get("X-Total-Count") != "" {
		t.Fatalf("no parameters: every generation, newest first, no headers: %+v %v", all, hdr)
	}
	page, hdr, _ := list("?limit=1&offset=1")
	if len(page) != 1 || page[0].ID != ids[1] || hdr.Get("X-Total-Count") != "3" || !strings.Contains(hdr.Get("Link"), "next") {
		t.Fatalf("the generation 2 of 3 with the total: %+v %v", page, hdr)
	}
	mine, hdr, _ := list("?client=alpha&limit=10")
	if len(mine) != 2 || hdr.Get("X-Total-Count") != "2" || mine[0].ID != ids[2] || mine[1].ID != ids[0] {
		t.Fatalf("the client filter: %+v %v", mine, hdr)
	}
	if oldest, _, _ := list("?sort=created_at&limit=1"); len(oldest) != 1 || oldest[0].ID != ids[0] {
		t.Fatalf("oldest first: %+v", oldest)
	}
	for _, bad := range []string{"?sort=colour", "?limit=0", "?client=Not%20A%20Client"} {
		if _, _, code := list(bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
}

func TestImageEdits_AreListedNewestFirst(t *testing.T) {
	s := editServer(t)
	if rec := doJSON(t, s.Handler(), http.MethodGet, "/api/image-edits", nil); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("nothing yet: %d %s", rec.Code, rec.Body)
	}
	first := decodeEdit(t, postEdit(t, s, map[string]any{"prompt": "one", "client": "alpha"}, pngBytes(16, 9)))
	time.Sleep(1100 * time.Millisecond)
	second := decodeEdit(t, postEdit(t, s, map[string]any{"prompt": "two"}, pngBytes(16, 9)))
	deadline := time.Now().Add(3 * time.Second)
	var list []imageEditView
	for time.Now().Before(deadline) {
		list = nil
		_ = json.Unmarshal(doJSON(t, s.Handler(), http.MethodGet, "/api/image-edits", nil).Body.Bytes(), &list)
		if len(list) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("newest first: %+v", list)
	}
	// each as the read of one answers it
	one := decodeEdit(t, doJSON(t, s.Handler(), http.MethodGet, "/api/image-edits/"+first.ID, nil))
	if list[1].ID != one.ID || list[1].State != one.State || list[1].Client != one.Client {
		t.Fatalf("%+v vs %+v", list[1], one)
	}
	var mine []imageEditView
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/image-edits?client=alpha&limit=5", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &mine)
	if len(mine) != 1 || mine[0].ID != first.ID || rec.Header().Get("X-Total-Count") != "1" {
		t.Fatalf("%+v %v", mine, rec.Header())
	}
	// a video edit is not a picture edit
	if rec := doJSON(t, s.Handler(), http.MethodGet, "/api/video-edits", nil); strings.Contains(rec.Body.String(), first.ID) {
		t.Fatal("the picture edit is listed among the video edits")
	}
}
