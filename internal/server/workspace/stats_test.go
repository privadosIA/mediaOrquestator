package workspace

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

type statRun struct {
	id, project, gpu string
	state            RunState
	client           string // the client stored in the run ("" for none)
	created          time.Time
	waitS, durS      float64 // 0 durS: no start/finish timestamps
	events           []string
}

func ev(typ, phase string, ms int64) string {
	if typ == "phase_end" {
		return fmt.Sprintf(`{"type":"phase_end","scene_id":"a","phase":%q,"duration_ms":%d}`, phase, ms)
	}
	return fmt.Sprintf(`{"type":%q,"scene_id":"a","phase":%q}`, typ, phase)
}

func seedStat(t *testing.T, s *testServer, r statRun) {
	t.Helper()
	run := Run{ID: r.id, Project: r.project, State: r.state, GPU: r.gpu, Client: r.client, CreatedAt: r.created}
	if r.durS > 0 {
		st := r.created.Add(time.Duration(r.waitS * float64(time.Second)))
		fin := st.Add(time.Duration(r.durS * float64(time.Second)))
		run.StartedAt, run.FinishedAt = &st, &fin
	}
	if err := s.rm.hist.createRun(&run); err != nil {
		t.Fatal(err)
	}
	for i, l := range r.events {
		if err := s.rm.hist.appendEvent(r.id, i, l); err != nil {
			t.Fatal(err)
		}
	}
}

func getStats(t *testing.T, s *testServer, query string) (statsResult, string) {
	t.Helper()
	rec := call(s.Handler(), http.MethodGet, "/api/stats"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/stats%s: %d %s", query, rec.Code, rec.Body)
	}
	var out statsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out, rec.Body.String()
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

var d0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

const wide = "?since=2026-01-01&until=2026-12-31"

func TestStats_CountsStatesAndSuccessRate(t *testing.T) {
	s, _ := launchServer(t)
	for i, st := range []RunState{StateSucceeded, StateSucceeded, StateSucceeded, StateFailed} {
		seedStat(t, s, statRun{id: fmt.Sprintf("r%d", i), project: "demo", state: st, created: d0, durS: 10})
	}
	res, _ := getStats(t, s, wide)
	if res.Totals.Runs != 4 || res.Totals.ByState[string(StateSucceeded)] != 3 || res.Totals.ByState[string(StateFailed)] != 1 {
		t.Fatalf("totals: %+v", res.Totals)
	}
	if res.Totals.SuccessRate == nil || !near(*res.Totals.SuccessRate, 0.75) {
		t.Errorf("success rate = %v, want 0.75", res.Totals.SuccessRate)
	}
}

func TestStats_SuccessRateIgnoresRunsStillActive(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "ok", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "live", project: "demo", state: StateRunning, created: d0})
	res, _ := getStats(t, s, wide)
	if res.Totals.SuccessRate == nil || !near(*res.Totals.SuccessRate, 1) {
		t.Errorf("an active run must not lower the rate: %v", res.Totals.SuccessRate)
	}
	if res.Totals.ByState[string(StateRunning)] != 1 {
		t.Errorf("by_state: %+v", res.Totals.ByState)
	}
}

func TestStats_TimePerPhaseWithExecutionsAndReuses(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 40,
		events: []string{ev("phase_end", "render", 10000), ev("skipped", "tts", 0), ev("phase_end", "tts", 500)}})
	seedStat(t, s, statRun{id: "b", project: "demo", state: StateSucceeded, created: d0, durS: 40,
		events: []string{ev("phase_end", "render", 20000), ev("skipped", "render", 0)}})
	res, _ := getStats(t, s, wide)
	var render, tts *statsPhase
	for i := range res.Phases {
		switch res.Phases[i].Phase {
		case "render":
			render = &res.Phases[i]
		case "tts":
			tts = &res.Phases[i]
		}
	}
	if render == nil || render.Executions != 2 || render.Reused != 1 || !near(render.TotalSecs, 30) ||
		render.MeanSecs == nil || !near(*render.MeanSecs, 15) || render.P95Secs == nil || !near(*render.P95Secs, 20) {
		t.Fatalf("render: %+v", render)
	}
	if tts == nil || tts.Executions != 1 || tts.Reused != 1 {
		t.Errorf("tts: %+v", tts)
	}
	if res.Phases[0].Phase != "tts" {
		t.Errorf("phases must follow the pipeline order, got %v first", res.Phases[0].Phase)
	}
}

func TestStats_CacheShare(t *testing.T) {
	s, _ := launchServer(t)
	var evs []string
	for i := 0; i < 6; i++ {
		evs = append(evs, ev("phase_end", "render", 1000))
	}
	for i := 0; i < 4; i++ {
		evs = append(evs, ev("skipped", "clip", 0))
	}
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 9, events: evs})
	res, _ := getStats(t, s, wide)
	if res.Cache.Executed != 6 || res.Cache.Reused != 4 || res.Cache.ReuseShare == nil || !near(*res.Cache.ReuseShare, 0.4) {
		t.Errorf("cache: %+v", res.Cache)
	}
}

func TestStats_DurationFiguresAndQueueWait(t *testing.T) {
	s, _ := launchServer(t)
	for i, d := range []float64{10, 20, 30, 100} {
		seedStat(t, s, statRun{id: fmt.Sprintf("d%d", i), project: "demo", state: StateSucceeded, created: d0, waitS: 4, durS: d})
	}
	seedStat(t, s, statRun{id: "bad", project: "demo", state: StateFailed, created: d0, waitS: 4, durS: 1000})
	res, _ := getStats(t, s, wide)
	du := res.Duration
	if du.Runs != 4 || du.Mean == nil || !near(*du.Mean, 40) || !near(*du.Median, 25) || !near(*du.P95, 100) || !near(du.Total, 160) {
		t.Errorf("duration over succeeded runs only: %+v", du)
	}
	if res.Totals.QueueWaitS == nil || !near(*res.Totals.QueueWaitS, 4) {
		t.Errorf("queue wait = %v", res.Totals.QueueWaitS)
	}
	if !near(res.Totals.TotalSecs, 1160) {
		t.Errorf("total seconds over every run with times = %v", res.Totals.TotalSecs)
	}
}

func TestStats_RunsWithoutEventsOrTimesCountButDoNotDistort(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "ok", project: "demo", state: StateSucceeded, created: d0, durS: 30,
		events: []string{ev("phase_end", "render", 30000)}})
	seedStat(t, s, statRun{id: "ghost", project: "demo", state: StateInterrupted, created: d0})
	res, _ := getStats(t, s, wide)
	if res.Totals.Runs != 2 || res.Totals.ByState[string(StateInterrupted)] != 1 {
		t.Fatalf("totals: %+v", res.Totals)
	}
	if res.Duration.Runs != 1 || res.Duration.Mean == nil || !near(*res.Duration.Mean, 30) {
		t.Errorf("duration: %+v", res.Duration)
	}
	if len(res.Phases) != 1 || res.Phases[0].Executions != 1 {
		t.Errorf("phases: %+v", res.Phases)
	}
	if len(res.RunDurations) != 1 {
		t.Errorf("only runs with times appear in the duration series: %+v", res.RunDurations)
	}
}

func TestStats_FilterByProjectAndPeriod(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "in", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "other", project: "test", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "old", project: "demo", state: StateSucceeded, created: d0.Add(-72 * time.Hour), durS: 5})
	res, _ := getStats(t, s, "?project=demo&since=2026-10-04&until=2026-10-31")
	if res.Totals.Runs != 1 || res.Project != "demo" || len(res.ByProject) != 1 || res.ByProject[0].Name != "demo" {
		t.Errorf("filtered: %+v", res)
	}
	res, _ = getStats(t, s, wide)
	if res.Totals.Runs != 3 || len(res.ByProject) != 2 {
		t.Errorf("no filter: %+v", res.Totals)
	}
}

func TestStats_DefaultPeriodIsTheLastThirtyDays(t *testing.T) {
	s, _ := launchServer(t)
	now := time.Now().UTC()
	seedStat(t, s, statRun{id: "recent", project: "demo", state: StateSucceeded, created: now.Add(-24 * time.Hour), durS: 5})
	seedStat(t, s, statRun{id: "stale", project: "demo", state: StateSucceeded, created: now.Add(-45 * 24 * time.Hour), durS: 5})
	res, _ := getStats(t, s, "")
	if res.Totals.Runs != 1 {
		t.Errorf("default window: %d runs, want 1", res.Totals.Runs)
	}
	if d := res.Period.Until.Sub(res.Period.Since); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Errorf("period = %v", d)
	}
}

func TestStats_ByDayAndPerGPUAndSlowest(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, gpu: "1", created: d0, durS: 60})
	seedStat(t, s, statRun{id: "b", project: "demo", state: StateFailed, gpu: "0,1", created: d0, durS: 30})
	seedStat(t, s, statRun{id: "c", project: "demo", state: StateCancelled, gpu: "", created: d0.Add(24 * time.Hour), durS: 10})
	res, _ := getStats(t, s, wide)
	if len(res.ByDay) != 2 || res.ByDay[0].Date != "2026-10-05" || res.ByDay[0].Succeeded != 1 || res.ByDay[0].Failed != 1 ||
		!near(res.ByDay[0].Seconds, 90) || res.ByDay[1].Other != 1 {
		t.Errorf("by day: %+v", res.ByDay)
	}
	g := map[string]statsGroup{}
	for _, x := range res.ByGPU {
		g[x.Name] = x
	}
	if g["1"].Runs != 2 || g["0"].Runs != 1 || g[""].Runs != 1 || !near(g["1"].Seconds, 90) {
		t.Errorf("per GPU (a two-GPU run counts for both): %+v", g)
	}
	if g["1"].SuccessRate == nil || !near(*g["1"].SuccessRate, 0.5) {
		t.Errorf("GPU 1 success rate: %v", g["1"].SuccessRate)
	}
	if len(res.Slowest) != 3 || res.Slowest[0].ID != "a" || res.Slowest[2].ID != "c" {
		t.Errorf("slowest: %+v", res.Slowest)
	}
}

func TestStats_SlowestIsCappedAtTen(t *testing.T) {
	s, _ := launchServer(t)
	for i := 0; i < 15; i++ {
		seedStat(t, s, statRun{id: fmt.Sprintf("r%02d", i), project: "demo", state: StateSucceeded, created: d0, durS: float64(10 + i)})
	}
	res, _ := getStats(t, s, wide)
	if len(res.Slowest) != 10 || res.Slowest[0].ID != "r14" || len(res.RunDurations) != 15 {
		t.Errorf("slowest %d (first %s), series %d", len(res.Slowest), res.Slowest[0].ID, len(res.RunDurations))
	}
}

func TestStats_AnEmptyPeriodIsNotAnError(t *testing.T) {
	s, _ := launchServer(t)
	res, raw := getStats(t, s, wide)
	if res.Totals.Runs != 0 || res.Totals.SuccessRate != nil || res.Duration.Mean != nil {
		t.Errorf("empty: %+v", res)
	}
	for _, field := range []string{`"by_day":[]`, `"phases":[]`, `"by_project":[]`, `"by_gpu":[]`, `"slowest":[]`, `"run_durations":[]`} {
		if !strings.Contains(raw, field) {
			t.Errorf("an empty series must serialize as [] (%s): %s", field, raw)
		}
	}
}

func TestStats_BadRangesAreRefused(t *testing.T) {
	s, _ := launchServer(t)
	for name, q := range map[string]string{
		"since after until": "?since=2026-10-10&until=2026-10-01",
		"bad since":         "?since=yesterday",
		"bad until":         "?until=" + url.QueryEscape("10/10/2026"),
	} {
		rec := call(s.Handler(), http.MethodGet, "/api/stats"+q, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
}

func TestStats_RFC3339TimestampsAreAccepted(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	res, _ := getStats(t, s, "?since="+url.QueryEscape("2026-10-05T11:00:00Z")+"&until="+url.QueryEscape("2026-10-05T13:00:00Z"))
	if res.Totals.Runs != 1 {
		t.Errorf("runs = %d", res.Totals.Runs)
	}
}

func TestStats_GroupsAndFiltersByTheKindOfTheProject(t *testing.T) {
	s, _ := newProjectsServer(t)
	for id, mode := range map[string]string{"clip": "video", "fotos": "images", "canciones": "music"} {
		createProject(t, s, id)
		data, err := s.ProjectDefinition(id)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.Contains(text, "mode:") {
			text = regexp.MustCompile(`(?m)^(\s*)mode:.*$`).ReplaceAllString(text, "${1}mode: "+mode)
		} else {
			text = strings.Replace(text, "output:", "output:\n  mode: "+mode, 1)
		}
		if err := s.SaveProjectDefinition(id, []byte(text), "save"); err != nil {
			t.Fatal(err)
		}
	}
	seedStat(t, s, statRun{id: "a", project: "clip", state: StateSucceeded, created: d0, durS: 30})
	seedStat(t, s, statRun{id: "b", project: "clip", state: StateFailed, created: d0, durS: 10})
	seedStat(t, s, statRun{id: "c", project: "fotos", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "d", project: "canciones", state: StateSucceeded, created: d0, durS: 20})
	seedStat(t, s, statRun{id: "e", project: "borrado", state: StateSucceeded, created: d0, durS: 1})

	res, _ := getStats(t, s, wide)
	by := map[string]statsGroup{}
	for _, g := range res.ByMode {
		by[g.Name] = g
	}
	if by["video"].Runs != 2 || by["images"].Runs != 1 || by["music"].Runs != 1 || by[""].Runs != 1 {
		t.Fatalf("by mode: %+v", res.ByMode)
	}
	if by["video"].SuccessRate == nil || !near(*by["video"].SuccessRate, 0.5) || !near(by["video"].Seconds, 40) {
		t.Errorf("video group: %+v", by["video"])
	}
	for _, p := range res.Slowest {
		if p.ID == "a" && p.Mode != "video" {
			t.Errorf("a run says the kind of its project: %+v", p)
		}
	}

	only, _ := getStats(t, s, wide+"&mode=images,music")
	if only.Totals.Runs != 2 || only.Mode != "images,music" || len(only.ByMode) != 2 {
		t.Fatalf("filtered: runs=%d mode=%q groups=%+v", only.Totals.Runs, only.Mode, only.ByMode)
	}
	rec := call(s.Handler(), http.MethodGet, "/api/stats"+wide+"&mode=video,lava", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown mode is refused: %d", rec.Code)
	}
}

func TestStats_FilterByOrigin(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "d1", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "d2", project: "otro", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "m1", project: "mobile-20261022-gato", state: StateSucceeded, created: d0, durS: 5})

	if res, _ := getStats(t, s, wide+"&origin=mobile"); res.Totals.Runs != 1 || res.ByProject[0].Name != "mobile-20261022-gato" {
		t.Errorf("mobile: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide+"&origin=desktop"); res.Totals.Runs != 2 {
		t.Errorf("desktop: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide+"&origin=desktop,mobile"); res.Totals.Runs != 3 {
		t.Errorf("both: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide); res.Totals.Runs != 3 {
		t.Errorf("none given: %+v", res.Totals)
	}
	if rec := call(s.Handler(), http.MethodGet, "/api/stats"+wide+"&origin=tablet", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown origin: %d", rec.Code)
	}
}

func TestStats_FilterByServer(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	other := Run{ID: "b", Project: "demo", State: StateSucceeded, Server: "portatil", CreatedAt: d0}
	if err := s.rm.hist.createRun(&other); err != nil {
		t.Fatal(err)
	}
	if res, _ := getStats(t, s, wide); res.Totals.Runs != 2 {
		t.Errorf("all: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide+"&server=portatil"); res.Totals.Runs != 1 {
		t.Errorf("one server: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide+"&server=portatil,nadie"); res.Totals.Runs != 1 {
		t.Errorf("several: %+v", res.Totals)
	}
}

func TestStats_FilterAndGroupByClient(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "p1", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "v1", project: GenPrefix + "1", client: "video-studio", state: StateSucceeded, gpu: "0", created: d0, durS: 30})
	seedStat(t, s, statRun{id: "v2", project: GenPrefix + "2", client: "video-studio", state: StateFailed, gpu: "1", created: d0, durS: 10})
	seedStat(t, s, statRun{id: "e1", project: EditPrefix + "1", client: "photo-lab", state: StateSucceeded, created: d0, durS: 4})
	seedStat(t, s, statRun{id: "o1", project: GenPrefix + "old", state: StateSucceeded, created: d0, durS: 8}) // before clients

	all, _ := getStats(t, s, wide)
	by := map[string]statsGroup{}
	for _, g := range all.ByClient {
		by[g.Name] = g
	}
	if all.Totals.Runs != 5 || len(by) != 4 || by["video-studio"].Runs != 2 || by["photo-lab"].Runs != 1 || by["embedded"].Runs != 1 || by["unknown"].Runs != 1 {
		t.Fatalf("by client: %+v", all.ByClient)
	}
	if g := by["video-studio"]; g.SuccessRate == nil || !near(*g.SuccessRate, 0.5) || !near(g.Seconds, 40) {
		t.Errorf("video-studio group: %+v", g)
	}
	// the generations still count by kind and by GPU, and still are not projects of their own
	for _, g := range all.ByProject {
		if IsManagedProject(g.Name) {
			t.Errorf("a generation is in the table of projects: %+v", g)
		}
	}
	gpus := 0
	for _, g := range all.ByGPU {
		gpus += g.Runs
	}
	if gpus < 2 {
		t.Errorf("by gpu: %+v", all.ByGPU)
	}

	one, _ := getStats(t, s, wide+"&client=video-studio")
	if one.Totals.Runs != 2 || len(one.ByClient) != 1 || one.ByClient[0].Name != "video-studio" {
		t.Errorf("one client: runs %d, %+v", one.Totals.Runs, one.ByClient)
	}
	if res, _ := getStats(t, s, wide+"&client=embedded"); res.Totals.Runs != 1 {
		t.Errorf("embedded: %+v", res.Totals)
	}
	if res, _ := getStats(t, s, wide+"&client=video-studio,unknown"); res.Totals.Runs != 3 || len(res.ByClient) != 2 {
		t.Errorf("two clients: runs %d, %+v", res.Totals.Runs, res.ByClient)
	}
	for _, bad := range []string{"Video%20Studio", "a_b"} {
		if rec := call(s.Handler(), http.MethodGet, "/api/stats"+wide+"&client="+bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("client %s: %d", bad, rec.Code)
		}
	}
}

// The runs of the music-motion pieces (projects api-mm-) are a separate world: without a mode they are not counted,
// the mode `animation` brings them in alone, and the three other modes add up to the figures of no mode.
func TestStats_AnimationsAreLeftOutUnlessAsked(t *testing.T) {
	s, _ := newProjectsServer(t)
	for id, mode := range map[string]string{"clip": "video", "fotos": "images", "canciones": "music"} {
		createProject(t, s, id)
		data, err := s.ProjectDefinition(id)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.Contains(text, "mode:") {
			text = regexp.MustCompile(`(?m)^(\s*)mode:.*$`).ReplaceAllString(text, "${1}mode: "+mode)
		} else {
			text = strings.Replace(text, "output:", "output:\n  mode: "+mode, 1)
		}
		if err := s.SaveProjectDefinition(id, []byte(text), "save"); err != nil {
			t.Fatal(err)
		}
	}
	seedStat(t, s, statRun{id: "a", project: "clip", state: StateSucceeded, created: d0, durS: 30})
	seedStat(t, s, statRun{id: "c", project: "fotos", state: StateSucceeded, created: d0, durS: 5})
	seedStat(t, s, statRun{id: "d", project: "canciones", state: StateSucceeded, created: d0, durS: 20})
	seedStat(t, s, statRun{id: "m1", project: "api-mm-20261101-000000-aaaaaa", state: StateSucceeded, created: d0, durS: 100})
	seedStat(t, s, statRun{id: "m2", project: "api-mm-20261101-000001-bbbbbb", state: StateFailed, created: d0, durS: 50})

	all, _ := getStats(t, s, wide)
	if all.Totals.Runs != 3 || all.Mode != "" {
		t.Fatalf("no mode leaves the animations out: runs=%d mode=%q", all.Totals.Runs, all.Mode)
	}
	for _, g := range all.ByMode {
		if g.Name == "animation" {
			t.Errorf("no animation group without the mode: %+v", all.ByMode)
		}
	}
	three, _ := getStats(t, s, wide+"&mode=video,images,music")
	if three.Totals.Runs != all.Totals.Runs || !near(three.Totals.TotalSecs, all.Totals.TotalSecs) {
		t.Errorf("video+images+music must equal no mode: %+v vs %+v", three.Totals, all.Totals)
	}

	anim, _ := getStats(t, s, wide+"&mode=animation")
	if anim.Totals.Runs != 2 || anim.Mode != "animation" || len(anim.ByMode) != 1 || anim.ByMode[0].Name != "animation" {
		t.Fatalf("animation alone: runs=%d groups=%+v", anim.Totals.Runs, anim.ByMode)
	}
	for _, p := range anim.RunDurations {
		if p.Mode != "animation" {
			t.Errorf("a run point says animation: %+v", p)
		}
	}
	mixed, _ := getStats(t, s, wide+"&mode=music,animation")
	if mixed.Totals.Runs != 3 {
		t.Errorf("a list that includes animation brings them in: %d", mixed.Totals.Runs)
	}

	// The run history says the mode too, and filters with it.
	rec := call(s.Handler(), http.MethodGet, "/api/runs?mode=animation", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/runs?mode=animation: %d %s", rec.Code, rec.Body)
	}
	var runs []RunView
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Mode != "animation" || runs[1].Mode != "animation" {
		t.Errorf("animation runs: %+v", runs)
	}
	rec = call(s.Handler(), http.MethodGet, "/api/runs?mode=video,images,music", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 3 {
		t.Errorf("the three other modes: %v %d", err, len(runs))
	}
	rec = call(s.Handler(), http.MethodGet, "/api/runs", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 5 {
		t.Errorf("no mode lists every run, as before: %v %d", err, len(runs))
	}
	if rec = call(s.Handler(), http.MethodGet, "/api/runs?mode=lava", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown mode is refused: %d", rec.Code)
	}
}
