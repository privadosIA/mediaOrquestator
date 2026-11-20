package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installStubBinaries drops executable piper/sd/ffmpeg/ffprobe stand-ins
// into a dir and prepends it to PATH, so the real pipeline runs end to
// end without GPUs or native tools (the repo's fake-binary approach,
// taken through actual subprocesses).
func installStubBinaries(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakebin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	stubs := map[string]string{
		// Piper: narration arrives on stdin; the adapter writes our
		// stdout into the .wav the pipeline pre-created.
		"piper": "#!/bin/sh\ncat >/dev/null\necho FAKEWAV\n",
		// ffprobe: report a fixed duration as JSON.
		"ffprobe": `#!/bin/sh
echo '{"format":{"duration":"2.5"}}'
`,
		// sd: write the PNG named by -o.
		"sd": `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
echo FAKEPNG > "$out"
`,
		// ffmpeg: its output file is always the last argument.
		"ffmpeg": `#!/bin/sh
for a in "$@"; do out="$a"; done
echo FAKEMEDIA > "$out"
`,
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestE2E_FullFlowAgainstRealPipeline(t *testing.T) {
	installStubBinaries(t)
	s, projects := newProjectsServer(t)
	t.Cleanup(s.rm.shutdown)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// 1. Create the project and save a real two-scene config (one
	//    narrated, one silent).
	createProject(t, s, "e2e")
	yaml := `project: e2e
output:
  mode: video
  file: out/e2e.mp4
defaults:
  model: models/m.safetensors
  voice: voices/v.onnx
  resolution: 640x360
  sampler: euler
  steps: 4
  fps: 30
scenes:
  - id: intro
    prompt: una ciudad
    seed: 42
    tts_text: Bienvenidos
    speed: 1.0
  - id: respiro
    prompt: niebla
    silence_seconds: 1.0
`
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/projects/e2e/config", map[string]string{"yaml": yaml})
	if rec.Code != http.StatusOK {
		t.Fatalf("save config: %d %s", rec.Code, rec.Body.String())
	}

	// 2. Launch a run (keep-tmp so artifacts and cache survive).
	id, _ := launchRun(t, s, "e2e", map[string]any{"keep_tmp": true})

	// 3. Follow the SSE stream to pipeline_end.
	resp, err := http.Get(ts.URL + "/api/runs/" + id + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	msgs := readSSE(t, resp.Body, 10*time.Second)
	evs := pipelineEvents(msgs)
	if len(evs) == 0 {
		t.Fatalf("no pipeline events; frames: %+v", msgs)
	}

	type ev struct {
		Type    string `json:"type"`
		SceneID string `json:"scene_id"`
		Phase   string `json:"phase"`
	}
	var seq []ev
	for _, raw := range evs {
		var e ev
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatalf("bad event %q: %v", raw, err)
		}
		seq = append(seq, e)
	}
	if seq[0].Type != "pipeline_start" || seq[len(seq)-1].Type != "pipeline_end" {
		t.Fatalf("stream must span pipeline_start..pipeline_end: %+v", seq)
	}
	phasesOf := func(scene string) []string {
		var out []string
		for _, e := range seq {
			if e.SceneID == scene && e.Type == "phase_start" {
				out = append(out, e.Phase)
			}
		}
		return out
	}
	wantIntro := []string{"tts", "probe", "render", "clip"}
	if got := phasesOf("intro"); strings.Join(got, ",") != strings.Join(wantIntro, ",") {
		t.Errorf("intro phases = %v, want %v", got, wantIntro)
	}
	// The silent scene must emit no tts/probe at all.
	wantSilent := []string{"render", "clip"}
	if got := phasesOf("respiro"); strings.Join(got, ",") != strings.Join(wantSilent, ",") {
		t.Errorf("respiro phases = %v, want %v", got, wantSilent)
	}
	if got := phasesOf("_global"); strings.Join(got, ",") != "concat" {
		t.Errorf("_global phases = %v, want [concat]", got)
	}

	// 4. The run is recorded as succeeded with the final artifact.
	runRec := waitState(t, s, id, StateSucceeded)
	if runRec.Artifact != "out/e2e.mp4" {
		t.Errorf("artifact = %q, want out/e2e.mp4", runRec.Artifact)
	}

	// 5. Artifacts are served: final video, an intermediate, and the
	//    kept cache manifest exists on disk.
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/projects/e2e/artifacts/out/e2e.mp4", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "FAKEMEDIA") {
		t.Errorf("final video: %d %q", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/projects/e2e/artifacts/tmp/scene_intro.wav", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "FAKEWAV") {
		t.Errorf("intermediate wav: %d %q", rec.Code, rec.Body.String())
	}

	// 6. The cache report now shows hits for the narrated scene.
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/projects/e2e/cache", nil)
	var cr CacheReport
	if err := json.Unmarshal(rec.Body.Bytes(), &cr); err != nil {
		t.Fatal(err)
	}
	if cr.Cold {
		t.Error("cache should be warm after a keep-tmp run")
	}
	for _, sc := range cr.Scenes {
		if sc.SceneID == "intro" {
			if sc.Phases["tts"] != PhaseHit || sc.Phases["render"] != PhaseHit || sc.Phases["clip"] != PhaseHit {
				t.Errorf("intro cache after run: %v", sc.Phases)
			}
		}
	}

	// 7. Switch to images mode and run again: render is reused from the
	//    shared cache, export produces the manifest.
	imgYaml := strings.Replace(yaml, "mode: video", "mode: images", 1)
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/projects/e2e/config", map[string]string{"yaml": imgYaml})
	if rec.Code != http.StatusOK {
		t.Fatalf("save images config: %d %s", rec.Code, rec.Body.String())
	}
	id2, _ := launchRun(t, s, "e2e", map[string]any{"keep_tmp": true})
	waitState(t, s, id2, StateSucceeded)

	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/projects/e2e/artifacts/out/e2e/manifest.json", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("manifest: %d %s", rec.Code, rec.Body.String())
	}
	var manifest struct {
		Project string `json:"project"`
		Mode    string `json:"mode"`
		Images  []struct {
			SceneID string `json:"scene_id"`
			File    string `json:"file"`
		} `json:"images"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Project != "e2e" || manifest.Mode != "images" || len(manifest.Images) != 2 {
		t.Errorf("manifest: %+v", manifest)
	}

	// 8. History lists both runs, newest first.
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/runs?project=e2e", nil)
	var list []Run
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != id2 || list[1].ID != id {
		t.Errorf("history wrong: %+v", list)
	}

	// Events were persisted verbatim for the finished run.
	lines, err := s.rm.hist.listEvents(id)
	if err != nil || len(lines) != len(evs) {
		t.Errorf("stored %d events, streamed %d (err=%v)", len(lines), len(evs), err)
	}

	_ = projects
}
