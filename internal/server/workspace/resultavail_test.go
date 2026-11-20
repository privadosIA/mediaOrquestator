package workspace

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func addRun(t *testing.T, s *testServer, id, project, artifact string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.rm.hist.createRun(&Run{ID: id, Project: project, State: "succeeded", Artifact: artifact, CreatedAt: now, StartedAt: &now, FinishedAt: &now}); err != nil {
		t.Fatal(err)
	}
}

func available(t *testing.T, s *testServer, id string) bool {
	t.Helper()
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/runs/"+id, nil)
	var out struct {
		Run struct {
			ResultAvailable bool `json:"result_available"`
		} `json:"run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %v", rec.Body, err)
	}
	return out.Run.ResultAvailable
}

// setImagesMode makes a project a picture project (the saved definition changes, not the file).
func setImagesMode(t *testing.T, s *testServer, id string) {
	t.Helper()
	raw, err := s.ProjectDefinition(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProjectDefinition(id, []byte(strings.Replace(string(raw), "mode: video", "mode: images", 1)), "save"); err != nil {
		t.Fatal(err)
	}
}

func TestResultAvailable_FollowsTheFilesOfEachMode(t *testing.T) {
	s, dir := newProjectsServer(t)
	createProject(t, s, "vid")
	os.MkdirAll(filepath.Join(dir, "vid", "out"), 0o755)
	os.WriteFile(filepath.Join(dir, "vid", "out", "vid.mp4"), []byte("x"), 0o644)
	addRun(t, s, "r-vid", "vid", "out/vid.mp4")
	if !available(t, s, "r-vid") {
		t.Error("the final video is there")
	}
	os.Remove(filepath.Join(dir, "vid", "out", "vid.mp4"))
	if available(t, s, "r-vid") {
		t.Error("the video and every clip are gone")
	}
	os.MkdirAll(filepath.Join(dir, "vid", "out", "clips"), 0o755)
	os.WriteFile(filepath.Join(dir, "vid", "out", "clips", "s1.mp4"), []byte("x"), 0o644)
	if !available(t, s, "r-vid") {
		t.Error("a kept clip is a result")
	}

	// a picture project
	createProject(t, s, "pics")
	setImagesMode(t, s, "pics")
	os.MkdirAll(filepath.Join(dir, "pics", "out", "pics"), 0o755)
	os.WriteFile(filepath.Join(dir, "pics", "out", "pics", "manifest.json"), []byte("{}"), 0o644)
	addRun(t, s, "r-pics", "pics", "out/pics/manifest.json")
	if available(t, s, "r-pics") {
		t.Error("a manifest with no picture is not a result")
	}
	os.WriteFile(filepath.Join(dir, "pics", "out", "pics", "s1_v1.png"), []byte("x"), 0o644)
	if !available(t, s, "r-pics") {
		t.Error("the picture is there")
	}
	os.Remove(filepath.Join(dir, "pics", "out", "pics", "s1_v1.png"))
	if available(t, s, "r-pics") {
		t.Error("the pictures were cleaned")
	}

	// a deleted project: the run stays, with no result
	os.RemoveAll(filepath.Join(dir, "vid"))
	if available(t, s, "r-vid") {
		t.Error("the project is gone")
	}
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/runs", nil)
	if !strings.Contains(rec.Body.String(), `"r-vid"`) {
		t.Error("the run must stay in the history")
	}
}

func TestResultAvailable_AVideoRunKeepsItsVideoWhenTheProjectIsSwitchedToImages(t *testing.T) {
	s, dir := newProjectsServer(t)
	createProject(t, s, "vid")
	os.MkdirAll(filepath.Join(dir, "vid", "out"), 0o755)
	os.WriteFile(filepath.Join(dir, "vid", "out", "vid.mp4"), []byte("x"), 0o644)
	addRun(t, s, "r-vid", "vid", "out/vid.mp4")
	setImagesMode(t, s, "vid")
	if !available(t, s, "r-vid") {
		t.Error("the run made a video and the video is still there")
	}
	os.Remove(filepath.Join(dir, "vid", "out", "vid.mp4"))
	if available(t, s, "r-vid") {
		t.Error("the video is gone")
	}
}

func TestResultAvailable_TheStatisticsTablesCarryIt(t *testing.T) {
	s, dir := newProjectsServer(t)
	createProject(t, s, "gone")
	addRun(t, s, "r-gone", "gone", "out/gone.mp4")
	os.RemoveAll(filepath.Join(dir, "gone"))
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/stats", nil)
	var st struct {
		Totals  struct{ Runs int } `json:"totals"`
		Slowest []struct {
			ID              string `json:"id"`
			ResultAvailable bool   `json:"result_available"`
		} `json:"slowest"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.Totals.Runs != 1 {
		t.Fatalf("%v %s", err, rec.Body)
	}
	for _, p := range st.Slowest {
		if p.ID == "r-gone" && p.ResultAvailable {
			t.Error("a run of a deleted project must be counted but not linked")
		}
	}
}
