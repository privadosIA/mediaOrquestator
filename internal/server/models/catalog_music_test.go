package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	idMusic17 = "music-acestep-1-7b-turbo"
	idMusic06 = "music-acestep-0-6b-turbo"
)

// musicEntries returns every music bundle of the catalog.
func musicEntries() []catalogEntry {
	var out []catalogEntry
	for _, e := range catalog {
		if e.Kind == "music" {
			out = append(out, e)
		}
	}
	return out
}

var measuredVRAM = regexp.MustCompile(`\d,\d GB de VRAM`)

func TestCatalogMusic_BundlesHaveTheFourRolesAndAreDocumented(t *testing.T) {
	entries := musicEntries()
	if len(entries) < 5 {
		t.Fatalf("expected the default, the light bundle and the three variants, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Kind != "music" || e.VRAMMB <= 0 || len(e.Files) != 4 || e.SizeBytes <= 0 {
			t.Fatalf("%s: kind %q vram %d files %d size %d", e.ID, e.Kind, e.VRAMMB, len(e.Files), e.SizeBytes)
		}
		roles := rolesByName(e)
		for _, want := range []string{"lm", "text_encoder", "dit", "vae"} {
			if roles[want] == "" {
				t.Errorf("%s: no %s file", e.ID, want)
			}
		}
		for _, f := range e.Files {
			if !strings.HasPrefix(f.URL, "https://huggingface.co/") || !strings.HasSuffix(f.Name, ".gguf") {
				t.Errorf("%s: %s must be an https Hugging Face .gguf", e.ID, f.Name)
			}
		}
		for _, want := range []string{"MIT", "tercero", "defaults.music"} {
			if !strings.Contains(e.Description, want) {
				t.Errorf("%s: the description must mention %q: %s", e.ID, want, e.Description)
			}
		}
		if !measuredVRAM.MatchString(e.Description) && !strings.Contains(e.Description, "pico de 8,5 GB") {
			t.Errorf("%s: the description must state the measured VRAM peak: %s", e.ID, e.Description)
		}
		if e.RequiresToken != "" {
			t.Errorf("%s: the GGUFs are public, requires_token = %q", e.ID, e.RequiresToken)
		}
	}
}

// The XL synthesis model exists in GGUF form and the engine supports it, so
// the catalog offers it (an earlier version of the project said otherwise).
func TestCatalogMusic_OffersAnXLSynthesisModel(t *testing.T) {
	for _, e := range musicEntries() {
		if strings.Contains(rolesByName(e)["dit"], "xl-") {
			return
		}
	}
	t.Error("no music bundle uses an XL synthesis model")
}

// Every bundle but the default differs from it in one file, so installing
// it after the default downloads only that file.
func TestCatalogMusic_VariantsDifferFromTheDefaultInOneFile(t *testing.T) {
	def := rolesByName(catalogEntryByID(t, idMusic17))
	for _, e := range musicEntries() {
		if e.ID == idMusic17 {
			continue
		}
		differing := 0
		for role, name := range rolesByName(e) {
			if def[role] != name {
				differing++
			}
		}
		if differing != 1 {
			t.Errorf("%s differs from the default bundle in %d files, want 1", e.ID, differing)
		}
	}
}

func TestCatalogMusic_SizesAreFourGGUFFilesWorth(t *testing.T) {
	for _, e := range musicEntries() {
		if e.SizeBytes < 3_000_000_000 || e.SizeBytes > 12_000_000_000 {
			t.Errorf("%s: size %d is implausible for four GGUF files", e.ID, e.SizeBytes)
		}
	}
}

func TestCatalogMusic_BundlesShareTheirSupportFiles(t *testing.T) {
	a, b := rolesByName(catalogEntryByID(t, idMusic17)), rolesByName(catalogEntryByID(t, idMusic06))
	for _, role := range []string{"text_encoder", "dit", "vae"} {
		if a[role] != b[role] {
			t.Errorf("%s must be the same file in both bundles: %q vs %q", role, a[role], b[role])
		}
	}
	if a["lm"] == b["lm"] {
		t.Error("the bundles differ in their language model")
	}
}

func TestCatalogMusic_InstalledOnlyWithEveryFileInModelsMusic(t *testing.T) {
	s := newAssetServer(t)
	e := catalogEntryByID(t, idMusic17)
	for i, f := range e.Files {
		if catalogEntryInstalled(t, s, idMusic17) {
			t.Fatalf("installed after only %d of 4 files", i)
		}
		writeAsset(t, s, "music", f.Name, 1)
	}
	if !catalogEntryInstalled(t, s, idMusic17) {
		t.Error("with the four files the bundle is installed")
	}
	// Removing a shared support file un-installs every bundle that lists it.
	rec := call(s.Handler(), "DELETE", "/api/assets/music/"+rolesByName(e)["text_encoder"], "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	for _, id := range []string{idMusic17, idMusic06} {
		if catalogEntryInstalled(t, s, id) {
			t.Errorf("%s must read as not installed without the text encoder", id)
		}
	}
}

func TestCatalogMusic_ListedPerKindWithItsDirectory(t *testing.T) {
	s := newAssetServer(t)
	writeAsset(t, s, "music", "vae-BF16.gguf", 5)
	rec := call(s.Handler(), "GET", "/api/assets", "")
	var out struct {
		Kinds []struct {
			Kind   string
			Dir    string
			Assets []struct{ Name string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, k := range out.Kinds {
		if k.Kind == "music" {
			if k.Dir != filepath.Join("models", "music") || len(k.Assets) != 1 || k.Assets[0].Name != "vae-BF16.gguf" {
				t.Fatalf("music kind = %+v", k)
			}
			return
		}
	}
	t.Fatal("no music kind in the asset listing")
}

func TestCatalogMusic_NamesAreConfinedToTheDirectory(t *testing.T) {
	s := newAssetServer(t)
	for _, name := range []string{"..%2Fx.gguf", "weights.safetensors"} {
		rec := call(s.Handler(), "DELETE", "/api/assets/music/"+name, "")
		if rec.Code == http.StatusNoContent || rec.Code >= 500 {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
}

func TestCatalogMusic_SecondBundleFetchesOnlyItsLanguageModel(t *testing.T) {
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
		if local[i].ID == idMusic17 || local[i].ID == idMusic06 {
			files := append([]catalogFile{}, local[i].Files...)
			for j := range files {
				files[j].URL = src.URL + "/" + files[j].Name
			}
			local[i].Files = files
		}
	}
	catalog = local

	_, d := postDownload(t, s, `{"id":"`+idMusic17+`"}`)
	if f := waitDL(t, s, d.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("1.7B = %+v", f)
	}
	rec, d2 := postDownload(t, s, `{"id":"`+idMusic06+`"}`)
	if rec.Code != 202 {
		t.Fatalf("0.6B: %d %s", rec.Code, rec.Body)
	}
	if len(d2.Files) != 1 || d2.Files[0] != rolesByName(catalogEntryByID(t, idMusic06))["lm"] {
		t.Errorf("only the language model is missing, got %v", d2.Files)
	}
	if f := waitDL(t, s, d2.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("0.6B = %+v", f)
	}
	if hits.Load() != 5 {
		t.Errorf("total fetches = %d, want 5 (4 + 1)", hits.Load())
	}
	if !catalogEntryInstalled(t, s, idMusic06) {
		t.Error("the 0.6B bundle must read as installed after its download")
	}
}

func TestCatalogMusic_XLVariantAfterTheDefaultFetchesOnlyTheXLModel(t *testing.T) {
	const idXL = "music-acestep-xl-turbo"
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
		if local[i].ID == idMusic17 || local[i].ID == idXL {
			files := append([]catalogFile{}, local[i].Files...)
			for j := range files {
				files[j].URL = src.URL + "/" + files[j].Name
			}
			local[i].Files = files
		}
	}
	catalog = local

	_, d := postDownload(t, s, `{"id":"`+idMusic17+`"}`)
	if f := waitDL(t, s, d.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("default = %+v", f)
	}
	rec, d2 := postDownload(t, s, `{"id":"`+idXL+`"}`)
	if rec.Code != 202 {
		t.Fatalf("XL: %d %s", rec.Code, rec.Body)
	}
	if len(d2.Files) != 1 || d2.Files[0] != rolesByName(catalogEntryByID(t, idXL))["dit"] {
		t.Errorf("only the XL synthesis model is missing, got %v", d2.Files)
	}
	if f := waitDL(t, s, d2.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("XL = %+v", f)
	}
	if hits.Load() != 5 {
		t.Errorf("total fetches = %d, want 5 (4 + 1)", hits.Load())
	}
}
