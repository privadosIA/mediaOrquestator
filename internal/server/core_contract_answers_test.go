package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// More answers of the real handlers walked against the description (see contract_test.go for the checker). Each case
// is built so the optional fields of its schema appear at least once, or the case would prove only half of it.

func TestContract_TheProjectsFitTheirDescription(t *testing.T) {
	c := newContractChecker(t)
	s, _ := newProjectsServer(t)
	createProject(t, s, "demo")
	dir := s.projectDir("songs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes.yaml"), []byte("project: songs\noutput:\n  mode: music\ntracks:\n  - id: ballad\n    prompt: p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A project whose definition cannot be read is listed too, with the fields that are always sent left empty.
	broken := s.projectDir("broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "scenes.yaml"), []byte("project: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.workspace.ImportFolders(); err != nil { // the folders written above become projects, as the importer of the server does
		t.Fatal(err)
	}
	body := get(t, s, "/api/projects")
	expectFits(t, c, "/api/projects", body)
	for _, field := range []string{`"track_count"`, `"invalid":true`, `"error"`} {
		if !strings.Contains(body, field) {
			t.Errorf("the case must carry %s, which is optional: %.400s", field, body)
		}
	}
}

func TestContract_TheProvidersAndTheRoutingFitTheirDescription(t *testing.T) {
	isolateSettings(t)
	c := newContractChecker(t)
	s := newTestServer(t)
	expectFits(t, c, "/api/providers", get(t, s, "/api/providers"))
	expectFits(t, c, "/api/routing", get(t, s, "/api/routing"))
}

func TestContract_TheDatabaseAndMonitoringSettingsFitTheirDescription(t *testing.T) {
	isolateSettings(t)
	c := newContractChecker(t)
	s := newTestServer(t)
	expectFits(t, c, "/api/settings/database", get(t, s, "/api/settings/database"))

	t.Run("monitoring with nothing saved", func(t *testing.T) {
		expectFits(t, c, "/api/settings/monitoring", get(t, s, "/api/settings/monitoring"))
	})
	t.Run("monitoring with an address, a user and a key saved", func(t *testing.T) {
		rec := doJSON(t, s.Handler(), "PUT", "/api/settings/monitoring/grafana", map[string]string{"address": "http://grafana.local:3000", "user": "admin", "secret": "k"})
		if rec.Code != 200 {
			t.Fatalf("saving the tool: %d %s", rec.Code, rec.Body)
		}
		body := get(t, s, "/api/settings/monitoring")
		expectFits(t, c, "/api/settings/monitoring", body)
		if !strings.Contains(body, `"address"`) || !strings.Contains(body, `"user"`) {
			t.Errorf("the case must carry the optional fields of a tool: %.300s", body)
		}
	})
}

func TestContract_TheRunsFitTheirDescription(t *testing.T) {
	c := newContractChecker(t)
	s, fe := launchServer(t)

	ok, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0", "keep_tmp": true})
	waitStart(t, fe)
	t.Run("a run that is going", func(t *testing.T) {
		waitState(t, s, ok, stateRunning)
		expectFits(t, c, "/api/runs/{id}", get(t, s, "/api/runs/"+ok))
	})
	fe.chanFor(ok) <- true
	waitState(t, s, ok, stateSucceeded)

	failed, _ := launchRun(t, s, "demo", nil)
	waitStart(t, fe)
	fe.chanFor(failed) <- false
	waitState(t, s, failed, stateFailed)

	t.Run("a run that succeeded and one that failed", func(t *testing.T) {
		list := get(t, s, "/api/runs")
		expectFits(t, c, "/api/runs", list)
		expectFits(t, c, "/api/runs/{id}", get(t, s, "/api/runs/"+failed))
		for _, field := range []string{`"started_at"`, `"finished_at"`, `"error"`} {
			if !strings.Contains(list, field) {
				t.Errorf("the case must carry %s, which is optional: %.400s", field, list)
			}
		}
	})
}

func TestContract_TheDownloadsFitTheirDescription(t *testing.T) {
	c := newContractChecker(t)
	s := newDLServer(t)
	src := fileServer(t, strings.Repeat("x", 1000))
	_, d := postDownload(t, s, fmt.Sprintf(`{"kind":"model","url":%q}`, src.URL+"/a/tiny.safetensors"))
	waitDL(t, s, d.ID, dlSucceeded, dlFailed)
	body := get(t, s, "/api/downloads")
	expectFits(t, c, "/api/downloads", body)
	expectFits(t, c, "/api/downloads/{id}", get(t, s, "/api/downloads/"+d.ID))
	if !strings.Contains(body, `"finished_at"`) || !strings.Contains(body, `"total"`) {
		t.Errorf("the case must carry the optional fields of a finished download: %.400s", body)
	}
}

func TestContract_AReviewWithItsFindingsFitsItsDescription(t *testing.T) {
	c := newContractChecker(t)
	s, _, _ := reviewServer(t)
	b := analyzeDemo(t, s, map[string]any{})
	if len(b.Findings) == 0 {
		t.Fatal("the case needs findings")
	}
	expectFits(t, c, "/api/reviews/{rid}", get(t, s, "/api/reviews/"+b.Review.ID))
}

func TestContract_TheStorageWithAMediaDirectoryFitsItsDescription(t *testing.T) {
	c := newContractChecker(t)
	s, _, _ := newMediaServer(t)
	createProject(t, s, "demo")
	body := get(t, s, "/api/storage")
	expectFits(t, c, "/api/storage", body)
	for _, field := range []string{`"media"`, `"dir"`, `"folder"`, `"free_bytes"`} {
		if !strings.Contains(body, field) {
			t.Errorf("the case must carry %s: %.400s", field, body)
		}
	}
}

func TestContract_TheModelsDirectoryFitsItsDescription(t *testing.T) {
	c := newContractChecker(t)
	s, _ := newModelsServer(t)
	for _, path := range []string{"/api/storage", "/api/environment"} {
		body := get(t, s, path)
		expectFits(t, c, path, body)
		for _, field := range []string{`"available"`, `"linked"`, `"free_bytes"`} {
			if !strings.Contains(body, field) {
				t.Errorf("%s must carry %s: %.300s", path, field, body)
			}
		}
	}
}
