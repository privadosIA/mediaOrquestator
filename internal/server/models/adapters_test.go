package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const idAdapterCNY = "music-adapter-chinese-new-year"

// installFolderAdapter writes a PEFT adapter folder like the official ones.
func installFolderAdapter(t *testing.T, s *testServer, name string) {
	t.Helper()
	writeAsset(t, s, "adapter", name+"/adapter_model.safetensors", 300)
	writeAsset(t, s, "adapter", name+"/adapter_config.json", 7)
}

func listAdapterUnits(t *testing.T, s *testServer) map[string]int64 {
	t.Helper()
	rec := call(s.Handler(), "GET", "/api/assets", "")
	var out struct {
		Kinds []struct {
			Kind   string
			Assets []struct {
				Name string
				Size int64
			}
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, k := range out.Kinds {
		if k.Kind == "adapter" {
			for _, a := range k.Assets {
				got[a.Name] = a.Size
			}
		}
	}
	return got
}

func TestAdapters_AFolderIsOneEntryWithItsTotalSize(t *testing.T) {
	s := newAssetServer(t)
	installFolderAdapter(t, s, "chinese-new-year")
	got := listAdapterUnits(t, s)
	if len(got) != 1 || got["chinese-new-year"] != 307 {
		t.Fatalf("units = %v", got)
	}
}

func TestAdapters_ASingleFileIsAnAdapterToo(t *testing.T) {
	s := newAssetServer(t)
	writeAsset(t, s, "adapter", "style.safetensors", 500)
	if got := listAdapterUnits(t, s); got["style.safetensors"] != 500 || len(got) != 1 {
		t.Fatalf("units = %v", got)
	}
}

func TestAdapters_LeftoversAndForeignFilesAreNotAdapters(t *testing.T) {
	s := newAssetServer(t)
	writeAsset(t, s, "adapter", "style.safetensors", 10)
	writeAsset(t, s, "adapter", "half.safetensors.partial", 10)                     // an interrupted download
	writeAsset(t, s, "adapter", "notes.txt", 10)                                    // not an adapter
	writeAsset(t, s, "adapter", "empty-folder/adapter_config.json", 10)             // a folder without the weights
	writeAsset(t, s, "adapter", "downloading/adapter_model.safetensors.partial", 5) // weights not there yet
	if got := listAdapterUnits(t, s); len(got) != 1 || got["style.safetensors"] != 10 {
		t.Fatalf("units = %v", got)
	}
}

func TestAdapters_AreNotListedAsMusicModelsAndTheReverse(t *testing.T) {
	s := newAssetServer(t)
	installFolderAdapter(t, s, "style")
	writeAsset(t, s, "music", "vae-BF16.gguf", 5)
	rec := call(s.Handler(), "GET", "/api/assets", "")
	var out struct {
		Kinds []struct {
			Kind   string
			Assets []struct{ Name string }
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	for _, k := range out.Kinds {
		if k.Kind == "music" && (len(k.Assets) != 1 || k.Assets[0].Name != "vae-BF16.gguf") {
			t.Errorf("music kind must list only its own files, not the adapters folder: %+v", k.Assets)
		}
	}
}

func TestAdapters_DeletingAFolderRemovesItWholeAndNothingElse(t *testing.T) {
	s := newAssetServer(t)
	installFolderAdapter(t, s, "style")
	installFolderAdapter(t, s, "other")
	writeAsset(t, s, "music", "vae-BF16.gguf", 5)
	rec := call(s.Handler(), "DELETE", "/api/assets/adapter/style", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	k, _ := FindKind("adapter")
	if _, err := os.Stat(filepath.Join(s.KindDir(k), "style")); !os.IsNotExist(err) {
		t.Fatal("the whole folder must be gone")
	}
	if got := listAdapterUnits(t, s); len(got) != 1 || got["other"] == 0 {
		t.Fatalf("the other adapter must stay: %v", got)
	}
	mk, _ := FindKind("music")
	if _, err := os.Stat(filepath.Join(s.KindDir(mk), "vae-BF16.gguf")); err != nil {
		t.Fatal("music models must not be touched")
	}
}

func TestAdapters_DeletingASingleFile(t *testing.T) {
	s := newAssetServer(t)
	writeAsset(t, s, "adapter", "style.safetensors", 10)
	if rec := call(s.Handler(), "DELETE", "/api/assets/adapter/style.safetensors", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if got := listAdapterUnits(t, s); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestAdapters_DeleteConfinesNamesAndAnswers404ForUnknownOnes(t *testing.T) {
	s := newAssetServer(t)
	installFolderAdapter(t, s, "style")
	for _, name := range []string{"..", ".hidden", "a%2Fb", "%2E%2E"} {
		rec := call(s.Handler(), "DELETE", "/api/assets/adapter/"+name, "")
		// the router itself may refuse (a redirect that cleans ".."); the point is
		// that it is never a success and nothing is removed
		if rec.Code == http.StatusNoContent || rec.Code >= 500 {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	if rec := call(s.Handler(), "DELETE", "/api/assets/adapter/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown adapter: %d", rec.Code)
	}
	if got := listAdapterUnits(t, s); got["style"] == 0 {
		t.Fatal("nothing may be removed by a rejected request")
	}
}

func TestAdapters_NamesAllowOneFolderLevelOnly(t *testing.T) {
	k, _ := FindKind("adapter")
	for name, ok := range map[string]bool{
		"style.safetensors":                true,
		"folder/adapter_model.safetensors": true,
		"folder/adapter_config.json":       true,
		"a/b/c.safetensors":                false,
		"../x.safetensors":                 false,
		"folder/../x.safetensors":          false,
		"folder/.hidden.safetensors":       false,
		"folder/notes.txt":                 false,
		"/abs.safetensors":                 false,
		"":                                 false,
	} {
		err := ValidateAssetName(k, name)
		if (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok=%v", name, err, ok)
		}
	}
	// other kinds keep refusing paths
	music, _ := FindKind("music")
	if ValidateAssetName(music, "folder/x.gguf") == nil {
		t.Error("only a kind of units may take a folder")
	}
}

func TestAdapters_CatalogEntryIsTwoFilesInOneFolder(t *testing.T) {
	e := catalogEntryByID(t, idAdapterCNY)
	if e.Kind != "adapter" || len(e.Files) != 2 || e.SizeBytes <= 0 || e.VRAMMB != 0 {
		t.Fatalf("%+v", e)
	}
	names := map[string]bool{}
	for _, f := range e.Files {
		names[f.Name] = true
		if !strings.HasPrefix(f.URL, "https://huggingface.co/") || !strings.HasPrefix(f.Name, "chinese-new-year/") {
			t.Errorf("%s: %s", f.Name, f.URL)
		}
	}
	if !names["chinese-new-year/adapter_model.safetensors"] || !names["chinese-new-year/adapter_config.json"] {
		t.Fatalf("an adapter folder needs its weights and its config: %v", names)
	}
	for _, want := range []string{"OpenRAIL", "adapter_scale", "defaults.music.adapter"} {
		if !strings.Contains(e.Description, want) {
			t.Errorf("the description must mention %q: %s", want, e.Description)
		}
	}
}

func TestAdapters_CatalogEntryIsInstalledOnlyWithBothFiles(t *testing.T) {
	s := newAssetServer(t)
	if catalogEntryInstalled(t, s, idAdapterCNY) {
		t.Fatal("not installed yet")
	}
	writeAsset(t, s, "adapter", "chinese-new-year/adapter_model.safetensors", 5)
	if catalogEntryInstalled(t, s, idAdapterCNY) {
		t.Fatal("one of two files is not an installed adapter")
	}
	writeAsset(t, s, "adapter", "chinese-new-year/adapter_config.json", 5)
	if !catalogEntryInstalled(t, s, idAdapterCNY) {
		t.Fatal("both files: installed")
	}
}

func TestAdapters_InstallingFromTheCatalogDownloadsIntoItsFolder(t *testing.T) {
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
		if local[i].ID == idAdapterCNY {
			files := append([]catalogFile{}, local[i].Files...)
			for j := range files {
				files[j].URL = src.URL + "/" + filepath.Base(files[j].Name)
			}
			local[i].Files = files
		}
	}
	catalog = local

	_, d := postDownload(t, s, `{"id":"`+idAdapterCNY+`"}`)
	if f := waitDL(t, s, d.ID, dlSucceeded, dlFailed); f.State != dlSucceeded {
		t.Fatalf("download = %+v", f)
	}
	k, _ := FindKind("adapter")
	for _, f := range []string{"adapter_model.safetensors", "adapter_config.json"} {
		if b, err := os.ReadFile(filepath.Join(s.KindDir(k), "chinese-new-year", f)); err != nil || !strings.HasPrefix(string(b), "payload-") {
			t.Fatalf("%s: %v %q", f, err, b)
		}
	}
	if !catalogEntryInstalled(t, s, idAdapterCNY) {
		t.Error("installed after its download")
	}
	if got := listAdapterUnits(t, s); len(got) != 1 || got["chinese-new-year"] == 0 {
		t.Fatalf("one adapter, with its size: %v", got)
	}
	// a second install has nothing left to fetch
	if rec, _ := postDownload(t, s, `{"id":"`+idAdapterCNY+`"}`); rec.Code != http.StatusConflict {
		t.Errorf("already installed: %d", rec.Code)
	}
	if hits.Load() != 2 {
		t.Errorf("fetches = %d, want 2", hits.Load())
	}
}

func TestAdapters_ACustomDownloadMayNameAFolderFileButNotTwoLevels(t *testing.T) {
	s := newDLServer(t)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("x")) }))
	t.Cleanup(src.Close)
	rec, _ := postDownload(t, s, `{"kind":"adapter","url":"`+src.URL+`/w","name":"a/b/c.safetensors"}`)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("two folder levels must be refused: %d %s", rec.Code, rec.Body)
	}
	rec, d := postDownload(t, s, `{"kind":"adapter","url":"`+src.URL+`/w","name":"mine/adapter_model.safetensors"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("one level is fine: %d %s", rec.Code, rec.Body)
	}
	waitDL(t, s, d.ID, dlSucceeded, dlFailed)
}

func TestAdapters_PartialsInAFolderAreSweptAtStart(t *testing.T) {
	s := newAssetServer(t)
	p := writeAsset(t, s, "adapter", "folder/adapter_model.safetensors.partial", 5)
	s.sweepPartials()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("an interrupted download inside a folder must be cleaned up")
	}
}
