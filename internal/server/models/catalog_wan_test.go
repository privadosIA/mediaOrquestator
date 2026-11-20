package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	id5BQ4  = "video-wan-2-2-ti2v-5b-q4"
	idA14B  = "video-wan-2-2-i2v-a14b-q4"
	a14bLow = "Wan2.2-I2V-A14B-LowNoise-Q4_K_S.gguf"
)

func TestCatalogWanA14B_HasTheFourRolesInTheVideoKind(t *testing.T) {
	e := catalogEntryByID(t, idA14B)
	if e.Kind != "video" || e.VRAMMB <= 0 || len(e.Files) != 4 {
		t.Fatalf("kind %q vram %d files %d", e.Kind, e.VRAMMB, len(e.Files))
	}
	roles := rolesByName(e)
	for _, want := range []string{"high_noise_model", "model", "text_encoder", "vae"} {
		if roles[want] == "" {
			t.Errorf("no %s file", want)
		}
	}
	if !strings.Contains(roles["high_noise_model"], "HighNoise") || roles["model"] != a14bLow {
		t.Errorf("the experts are mixed up: high=%q low=%q", roles["high_noise_model"], roles["model"])
	}
	for _, f := range e.Files {
		if !strings.HasPrefix(f.URL, "https://") {
			t.Errorf("%s must be an https URL", f.Name)
		}
	}
	for _, want := range []string{"684", "10,3", "21,1", "tercero", "Apache"} {
		if !strings.Contains(e.Description, want) {
			t.Errorf("the description must state the measurements and the license (%q): %s", want, e.Description)
		}
	}
	if e.RequiresToken != "" {
		t.Errorf("the A14B GGUFs are public, requires_token = %q", e.RequiresToken)
	}
}

func TestCatalogWanA14B_SharesTheTextEncoderWithTheFiveB_AndNotTheVAE(t *testing.T) {
	a, b := rolesByName(catalogEntryByID(t, id5BQ4)), rolesByName(catalogEntryByID(t, idA14B))
	if a["text_encoder"] != b["text_encoder"] {
		t.Errorf("the umt5 encoder must be the same file in both bundles: %q vs %q", a["text_encoder"], b["text_encoder"])
	}
	if a["vae"] == b["vae"] {
		t.Errorf("the A14B uses the Wan 2.1 VAE, not the 5B's %q", a["vae"])
	}
}

func TestCatalogWanA14B_InstalledOnlyWithEveryFileInModelsVideo(t *testing.T) {
	s := newAssetServer(t)
	e := catalogEntryByID(t, idA14B)
	installed := func() bool {
		return catalogEntryInstalled(t, s, idA14B)
	}
	for i, f := range e.Files {
		if installed() {
			t.Fatalf("installed after only %d of 4 files", i)
		}
		writeAsset(t, s, "video", f.Name, 1)
	}
	if !installed() {
		t.Error("with the four files the bundle is installed")
	}
}

func TestCatalogWanA14B_AfterTheFiveBOnlyThreeFilesAreFetched(t *testing.T) {
	s := newDLServer(t)
	var hits atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("payload-" + filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(src.Close)
	saved := catalog
	t.Cleanup(func() { catalog = saved })
	local := append([]catalogEntry{}, catalog...)
	for i := range local {
		if local[i].ID == id5BQ4 || local[i].ID == idA14B {
			files := append([]catalogFile{}, local[i].Files...)
			for j := range files {
				files[j].URL = src.URL + "/" + files[j].Name
			}
			local[i].Files = files
		}
	}
	catalog = local

	_, d := postDownload(t, s, `{"id":"`+id5BQ4+`"}`)
	if f := waitDL(t, s, d.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("5B = %+v", f)
	}
	rec, d2 := postDownload(t, s, `{"id":"`+idA14B+`"}`)
	if rec.Code != 202 {
		t.Fatalf("A14B: %d %s", rec.Code, rec.Body)
	}
	if len(d2.Files) != 3 {
		t.Errorf("the A14B must fetch its two experts and the Wan 2.1 VAE, not the shared encoder: %v", d2.Files)
	}
	for _, f := range d2.Files {
		if f == rolesByName(catalogEntryByID(t, idA14B))["text_encoder"] {
			t.Errorf("the shared text encoder was queued again")
		}
	}
	if f := waitDL(t, s, d2.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("A14B = %+v", f)
	}
	if hits.Load() != 6 {
		t.Errorf("total fetches = %d, want 6 (3 + 3)", hits.Load())
	}
	if !catalogEntryInstalled(t, s, idA14B) {
		t.Error("the A14B must read as installed after its download")
	}
}

func catalogEntryInstalled(t *testing.T, s *testServer, id string) bool {
	t.Helper()
	rec := call(s.Handler(), "GET", "/api/assets/catalog", "")
	var out struct{ Entries []catalogEntry }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, x := range out.Entries {
		if x.ID == id {
			return x.Installed
		}
	}
	t.Fatalf("%s missing from the served catalog", id)
	return false
}
