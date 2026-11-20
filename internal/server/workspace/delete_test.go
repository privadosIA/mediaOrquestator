package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedRun stores a run with n events directly in the history.
func seedRun(t *testing.T, s *testServer, id, project string, state RunState, created time.Time, events int) {
	t.Helper()
	r := Run{ID: id, Project: project, State: state, CreatedAt: created}
	if err := s.rm.hist.createRun(&r); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < events; i++ {
		if err := s.rm.hist.appendEvent(id, i, fmt.Sprintf(`{"type":"phase_start","seq":%d}`, i)); err != nil {
			t.Fatal(err)
		}
	}
}

func runExists(s *testServer, id string) bool {
	_, err := s.rm.hist.getRun(id)
	return err == nil
}

func eventCount(t *testing.T, s *testServer, id string) int {
	t.Helper()
	lines, err := s.rm.hist.listEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	return len(lines)
}

type batchResult struct {
	Deleted       int      `json:"deleted"`
	SkippedActive []string `json:"skipped_active"`
}

func postDelete(t *testing.T, s *testServer, body any) (int, batchResult, string) {
	t.Helper()
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/runs/delete", body)
	var out batchResult
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad batch answer: %v: %s", err, rec.Body)
		}
	}
	return rec.Code, out, rec.Body.String()
}

func deleteRun(s *testServer, id string) *httptest.ResponseRecorder {
	return call(s.Handler(), http.MethodDelete, "/api/runs/"+id, "")
}

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestRunDelete_FinishedRunGoesWithItsEventsButItsFilesStay(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "run-ok", "demo", StateSucceeded, t0, 5)
	out := filepath.Join(s.rm.projectsDir, "demo", "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(out, "x.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := deleteRun(s, "run-ok")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("DELETE: %d %s", rec.Code, rec.Body)
	}
	if runExists(s, "run-ok") || eventCount(t, s, "run-ok") != 0 {
		t.Error("the run and its events must be gone")
	}
	if _, err := os.Stat(video); err != nil {
		t.Errorf("the generated file must stay: %v", err)
	}
}

func TestRunDelete_ADeletedRunIsGoneForEveryEndpoint(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "run-x", "demo", StateFailed, t0, 3)
	deleteRun(s, "run-x")
	for _, path := range []string{"/api/runs/run-x", "/api/runs/run-x/events"} {
		rec := doJSON(t, s.Handler(), http.MethodGet, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", path, rec.Code)
		}
	}
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/runs", nil)
	if strings.Contains(rec.Body.String(), "run-x") {
		t.Errorf("the list still shows it: %s", rec.Body)
	}
	if rec := deleteRun(s, "run-x"); rec.Code != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", rec.Code)
	}
}

func TestRunDelete_ActiveRunsAreRefusedAndUnchanged(t *testing.T) {
	s, fe := launchServer(t)
	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0"})
	waitStart(t, fe)
	rec := deleteRun(s, id)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cancel") {
		t.Errorf("DELETE running: %d %s", rec.Code, rec.Body)
	}
	if !runExists(s, id) || eventCount(t, s, id) == 0 {
		t.Errorf("a refused delete must change nothing")
	}
	fe.chanFor(id) <- true
}

func TestRunDelete_AQueuedRunIsRefusedToo(t *testing.T) {
	s, fe := launchServer(t)
	createProject(t, s, "other")
	first, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0"})
	waitStart(t, fe)
	second, pos := launchRun(t, s, "other", map[string]any{"gpu": "0"})
	if pos == 0 {
		t.Skip("second run did not queue")
	}
	if rec := deleteRun(s, second); rec.Code != http.StatusConflict {
		t.Errorf("DELETE queued: %d %s", rec.Code, rec.Body)
	}
	if !runExists(s, second) {
		t.Error("the queued run must still exist")
	}
	fe.chanFor(first) <- true
	waitStart(t, fe)
	fe.chanFor(second) <- true
	waitState(t, s, second, StateSucceeded)
}

func TestRunDelete_BatchSkipsActiveRunsAndDeletesTheRest(t *testing.T) {
	s, fe := launchServer(t)
	seedRun(t, s, "a", "demo", StateSucceeded, t0, 2)
	seedRun(t, s, "b", "demo", StateFailed, t0, 2)
	live, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0"})
	waitStart(t, fe)

	code, res, body := postDelete(t, s, map[string]any{"ids": []string{"a", live, "b", "missing"}})
	if code != http.StatusOK || res.Deleted != 2 || len(res.SkippedActive) != 1 || res.SkippedActive[0] != live {
		t.Fatalf("batch: %d %s", code, body)
	}
	if runExists(s, "a") || runExists(s, "b") || !runExists(s, live) {
		t.Error("only the finished runs may go")
	}
	fe.chanFor(live) <- true
}

func TestRunDelete_FilterByProjectStatesAndDate(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "t-fail", "test", StateFailed, t0, 1)
	seedRun(t, s, "t-ok", "test", StateSucceeded, t0, 1)
	seedRun(t, s, "d-fail", "demo", StateFailed, t0, 1)
	seedRun(t, s, "t-old", "test", StateSucceeded, t0.Add(-48*time.Hour), 1)

	code, res, body := postDelete(t, s, map[string]any{"project": "test", "states": []string{"failed"}})
	if code != 200 || res.Deleted != 1 || runExists(s, "t-fail") || !runExists(s, "t-ok") || !runExists(s, "d-fail") {
		t.Fatalf("project+states: %d %s", code, body)
	}
	code, res, body = postDelete(t, s, map[string]any{"before": t0.Add(-time.Hour).Format(time.RFC3339)})
	if code != 200 || res.Deleted != 1 || runExists(s, "t-old") || !runExists(s, "t-ok") {
		t.Fatalf("before: %d %s", code, body)
	}
	code, res, body = postDelete(t, s, map[string]any{"project": "test"})
	if code != 200 || res.Deleted != 1 || runExists(s, "t-ok") || !runExists(s, "d-fail") {
		t.Fatalf("project only: %d %s", code, body)
	}
}

func TestRunDelete_NothingMatchingIsNotAnError(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "keep", "demo", StateSucceeded, t0, 1)
	code, res, body := postDelete(t, s, map[string]any{"project": "nobody"})
	if code != 200 || res.Deleted != 0 || res.SkippedActive == nil || len(res.SkippedActive) != 0 {
		t.Fatalf("%d %s", code, body)
	}
	if !strings.Contains(body, `"skipped_active":[]`) {
		t.Errorf("skipped_active must serialize as an empty list: %s", body)
	}
	if !runExists(s, "keep") {
		t.Error("nothing may change")
	}
}

func TestRunDelete_BadRequestsDeleteNothing(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "keep", "demo", StateSucceeded, t0, 1)
	for name, body := range map[string]any{
		"ids and a filter": map[string]any{"ids": []string{"keep"}, "project": "demo"},
		"neither":          map[string]any{},
		"an active state":  map[string]any{"states": []string{"running"}},
		"an unknown state": map[string]any{"states": []string{"weird"}},
		"a malformed date": map[string]any{"before": "yesterday"},
		"ids with a state": map[string]any{"ids": []string{"keep"}, "states": []string{"failed"}},
	} {
		if code, _, b := postDelete(t, s, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, code, b)
		}
	}
	for _, raw := range []string{"", "not json", "[]"} {
		rec := call(s.Handler(), http.MethodPost, "/api/runs/delete", raw)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: %d, want 400", raw, rec.Code)
		}
	}
	if !runExists(s, "keep") || eventCount(t, s, "keep") != 1 {
		t.Error("refused requests must delete nothing")
	}
}

func TestRunDelete_ALiveRunIsProtectedEvenIfTheDatabaseSaysFinished(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "ghost", "demo", StateSucceeded, t0, 2)
	s.rm.mu.Lock()
	s.rm.runs["ghost"] = &run{rec: Run{ID: "ghost", State: StateRunning}}
	s.rm.mu.Unlock()
	t.Cleanup(func() {
		s.rm.mu.Lock()
		delete(s.rm.runs, "ghost")
		s.rm.mu.Unlock()
	})

	if rec := deleteRun(s, "ghost"); rec.Code != http.StatusConflict {
		t.Errorf("DELETE: %d, want 409", rec.Code)
	}
	code, res, _ := postDelete(t, s, map[string]any{"project": "demo"})
	if code != 200 || res.Deleted != 0 || len(res.SkippedActive) != 1 {
		t.Errorf("filter: %d %+v", code, res)
	}
	if !runExists(s, "ghost") || eventCount(t, s, "ghost") != 2 {
		t.Error("a live run must be untouched")
	}
}

func TestRunDelete_AFailureLeavesNothingHalfDeleted(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "a", "demo", StateSucceeded, t0, 4)
	seedRun(t, s, "b", "demo", StateSucceeded, t0, 4)
	s.rm.hist.afterEventsDeleted = func() error { return errors.New("disk full") }

	if rec := deleteRun(s, "a"); rec.Code != http.StatusInternalServerError {
		t.Errorf("single: %d, want 500", rec.Code)
	}
	if code, _, _ := postDelete(t, s, map[string]any{"project": "demo"}); code != http.StatusInternalServerError {
		t.Errorf("batch: %d, want 500", code)
	}
	for _, id := range []string{"a", "b"} {
		if !runExists(s, id) || eventCount(t, s, id) != 4 {
			t.Errorf("%s must keep the run and all its events after a failure", id)
		}
	}
	s.rm.hist.afterEventsDeleted = nil
	if code, res, _ := postDelete(t, s, map[string]any{"project": "demo"}); code != 200 || res.Deleted != 2 {
		t.Errorf("after recovering: %d %+v", code, res)
	}
}

func TestRunDelete_ABigBatchIsChunked(t *testing.T) {
	s, _ := launchServer(t)
	const n = 1300 // more than one IN list
	for i := 0; i < n; i++ {
		seedRun(t, s, fmt.Sprintf("bulk-%04d", i), "bulk", StateSucceeded, t0, 0)
	}
	seedRun(t, s, "bulk-keep", "other", StateSucceeded, t0, 1)
	code, res, body := postDelete(t, s, map[string]any{"project": "bulk"})
	if code != 200 || res.Deleted != n {
		t.Fatalf("%d %s", code, body)
	}
	if !runExists(s, "bulk-keep") {
		t.Error("another project's run must stay")
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, fmt.Sprintf("bulk-%04d", i))
	}
	if code, res, _ := postDelete(t, s, map[string]any{"ids": ids}); code != 200 || res.Deleted != 0 {
		t.Errorf("by ids when already gone: %d %+v", code, res)
	}
}

// A run the database still calls queued or running is active even when the
// manager does not hold it (a stale row): it is never deleted, by id or filter.
func TestRunDelete_ARunTheDatabaseCallsActiveIsNeverDeleted(t *testing.T) {
	s, _ := launchServer(t)
	seedRun(t, s, "stale-run", "demo", StateRunning, t0, 2)
	seedRun(t, s, "stale-queue", "demo", StateQueued, t0, 1)
	seedRun(t, s, "done", "demo", StateSucceeded, t0, 1)

	if rec := deleteRun(s, "stale-run"); rec.Code != http.StatusConflict {
		t.Errorf("DELETE stale running: %d, want 409", rec.Code)
	}
	code, res, body := postDelete(t, s, map[string]any{"ids": []string{"stale-run", "stale-queue", "done"}})
	if code != 200 || res.Deleted != 1 || len(res.SkippedActive) != 2 {
		t.Fatalf("by ids: %d %s", code, body)
	}
	code, res, body = postDelete(t, s, map[string]any{"project": "demo"})
	if code != 200 || res.Deleted != 0 {
		t.Fatalf("by filter must not select active rows: %d %s", code, body)
	}
	for _, id := range []string{"stale-run", "stale-queue"} {
		if !runExists(s, id) {
			t.Errorf("%s must still exist", id)
		}
	}
	if eventCount(t, s, "stale-run") != 2 {
		t.Error("its events must be intact")
	}
}

func TestRunDelete_WithFilesRemovesTheOutputFolderAndTmp(t *testing.T) {
	s, _ := launchServer(t)
	proj := filepath.Join(s.rm.projectsDir, "demo")
	keep := map[string]string{
		filepath.Join(proj, "out", "other", "a.mp4"): "other output",
		filepath.Join(proj, "scenes.yaml"):           "yaml",
	}
	thumb := filepath.Join(proj, "tmp", "scene_a.png") // the editor's thumbnail
	if err := os.MkdirAll(filepath.Dir(thumb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumb, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(proj, "out", "demo", "final.mp4")
	for p, c := range keep {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(gone), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gone, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRun(t, s, "run-a", "demo", StateSucceeded, t0, 1)
	seedRun(t, s, "run-b", "demo", StateFailed, t0, 1) // no artifact
	for _, id := range []string{"run-a"} {
		r, _ := s.rm.hist.getRun(id)
		r.Artifact = "out/demo/final.mp4"
		if err := s.rm.hist.saveRun(r); err != nil {
			t.Fatal(err)
		}
	}

	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/runs/delete",
		map[string]any{"ids": []string{"run-a", "run-b"}, "delete_files": true})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"files_removed":2`) {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Dir(gone)); !os.IsNotExist(err) {
		t.Errorf("out/demo must be gone: %v", err)
	}
	if _, err := os.Stat(thumb); !os.IsNotExist(err) {
		t.Errorf("tmp/ (and so the scene thumbnail) must be gone too: %v", err)
	}
	for p := range keep {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must stay: %v", p, err)
		}
	}
}

func TestOutputPathOf(t *testing.T) {
	for in, want := range map[string]string{
		"out/demo/manifest.json": "out/demo",
		"out/demo/a/b.png":       "out/demo",
		"out/x.mp4":              "out/x.mp4",
		"":                       "",
		"out":                    "",
		"tmp/x/y":                "",
		"/out/x/y":               "",
		"out/../scenes.yaml":     "",
		"out/../../x/y":          "",
	} {
		if got := outputPathOf(in); got != want {
			t.Errorf("outputPathOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunDelete_WithFilesRemovesTheKeptClipsAndWithoutFilesKeepsThem(t *testing.T) {
	s, _ := launchServer(t)
	clip := filepath.Join(s.rm.projectsDir, "demo", "out", "clips", "a.mp4")
	if err := os.MkdirAll(filepath.Dir(clip), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clip, []byte("clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRun(t, s, "run-a", "demo", StateSucceeded, t0, 1)
	seedRun(t, s, "run-b", "demo", StateSucceeded, t0, 1)

	if rec := doJSON(t, s.Handler(), http.MethodPost, "/api/runs/delete", map[string]any{"ids": []string{"run-a"}}); rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(clip); err != nil {
		t.Fatalf("deleting a run without its files keeps the clips: %v", err)
	}
	if rec := doJSON(t, s.Handler(), http.MethodPost, "/api/runs/delete", map[string]any{"ids": []string{"run-b"}, "delete_files": true}); rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(clip); !os.IsNotExist(err) {
		t.Fatalf("deleting a run with its files removes the kept clips: %v", err)
	}
}
