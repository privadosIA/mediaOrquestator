package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// twoGPUs is what the fake nvidia-smi reports: a 12 GB and a 16 GB card.
func twoGPUs(used0, used1 int) func(context.Context) ([]GPUReading, error) {
	return func(context.Context) ([]GPUReading, error) {
		return []GPUReading{
			{Index: 0, UsedMB: used0, TotalMB: 12282, UtilPct: 5},
			{Index: 1, UsedMB: used1, TotalMB: 16311, UtilPct: 90},
		}, nil
	}
}

func fixedRAM(used, total int) func() (int, int, error) {
	return func() (int, int, error) { return used, total, nil }
}

func samplesOf(t *testing.T, s *testServer, runID string) []RunSample {
	t.Helper()
	var rows []RunSample
	if err := s.rm.hist.db.Where("run_id = ?", runID).Order("ts, id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func getResources(t *testing.T, s *testServer, id string) (int, resourcesResult, string) {
	t.Helper()
	rec := call(s.Handler(), http.MethodGet, "/api/runs/"+id+"/resources", "")
	var out resourcesResult
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out, rec.Body.String()
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSampler_StoresARowPerOccupiedGPUWithTheMachinesRAM(t *testing.T) {
	s, _ := launchServer(t)
	smp := s.rm.sampler
	smp.readGPUs, smp.readRAM = twoGPUs(1000, 14000), fixedRAM(21000, 30000)
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateRunning, created: d0})

	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"1"})})
	rows := samplesOf(t, s, "r")
	if len(rows) != 1 || rows[0].GPU != 1 || rows[0].VRAMUsedMB != 14000 || rows[0].VRAMTotalMB != 16311 ||
		rows[0].UtilPct != 90 || rows[0].RAMUsedMB != 21000 || rows[0].RAMTotalMB != 30000 {
		t.Fatalf("pinned to GPU 1: %+v", rows)
	}

	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"0", "1"})})
	if n := len(samplesOf(t, s, "r")); n != 3 {
		t.Errorf("two GPUs add two rows, total %d want 3", n)
	}
	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{""})})
	if n := len(samplesOf(t, s, "r")); n != 5 {
		t.Errorf("a run without GPUs occupies every reported GPU: total %d want 5", n)
	}
}

func TestSampler_WithoutNvidiaSMIOnlyTheRAMIsSampled(t *testing.T) {
	s, _ := launchServer(t)
	smp := s.rm.sampler
	smp.readGPUs = func(context.Context) ([]GPUReading, error) { return nil, ErrNoNvidiaSmi }
	smp.readRAM = fixedRAM(8000, 16000)
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateRunning, created: d0})
	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"1"})})
	rows := samplesOf(t, s, "r")
	if len(rows) != 1 || rows[0].GPU != -1 || rows[0].RAMUsedMB != 8000 || rows[0].VRAMTotalMB != 0 {
		t.Fatalf("RAM-only row expected: %+v", rows)
	}
}

func TestSampler_AFailedGPUReadSkipsThatSampleOnly(t *testing.T) {
	s, _ := launchServer(t)
	smp := s.rm.sampler
	var calls atomic.Int32
	smp.readGPUs = func(ctx context.Context) ([]GPUReading, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("nvidia-smi hiccup")
		}
		return twoGPUs(1, 2)(ctx)
	}
	smp.readRAM = fixedRAM(1, 2)
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateRunning, created: d0})
	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"1"})})
	if n := len(samplesOf(t, s, "r")); n != 0 {
		t.Errorf("the failed sample must store nothing, got %d rows", n)
	}
	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"1"})})
	if n := len(samplesOf(t, s, "r")); n != 1 {
		t.Errorf("the next sample works: %d rows", n)
	}
}

func TestSampler_AnUnreadableRAMIsStoredAsZeroNotAsAFailure(t *testing.T) {
	s, _ := launchServer(t)
	smp := s.rm.sampler
	smp.readGPUs = twoGPUs(1, 2)
	smp.readRAM = func() (int, int, error) { return 0, 0, errors.New("no /proc") }
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateRunning, created: d0})
	smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"1"})})
	rows := samplesOf(t, s, "r")
	if len(rows) != 1 || rows[0].VRAMUsedMB != 2 || rows[0].RAMTotalMB != 0 {
		t.Fatalf("%+v", rows)
	}
}

func TestSampler_StopsAtTheRowCapAndTheRunReportsTruncation(t *testing.T) {
	s, _ := launchServer(t)
	smp := s.rm.sampler
	smp.readGPUs, smp.readRAM = twoGPUs(1, 2), fixedRAM(1, 2)
	smp.mu.Lock()
	smp.maxRows = 3
	smp.mu.Unlock()
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateRunning, created: d0})
	for i := 0; i < 5; i++ {
		smp.sample(context.Background(), []sampleTarget{targetOf("r", []string{"0", "1"})})
	}
	if n := len(samplesOf(t, s, "r")); n != 2 {
		t.Errorf("with a cap of 3 rows and 2 per tick only one tick fits, got %d rows", n)
	}
	// a one-row-per-tick run fills the cap exactly and is then marked truncated
	seedStat(t, s, statRun{id: "q", project: "demo", state: StateSucceeded, created: d0})
	for i := 0; i < 6; i++ {
		smp.sample(context.Background(), []sampleTarget{targetOf("q", []string{"1"})})
	}
	code, res, body := getResources(t, s, "q")
	if code != 200 || !res.Truncated || len(res.GPUs[0].Samples) != 3 {
		t.Errorf("truncated: %d %s", code, body)
	}
	if _, res, _ := getResources(t, s, "r"); res.Truncated {
		t.Error("a run below the cap is not truncated")
	}
}

func TestSampler_NothingIsReadWhileNoRunIsActive(t *testing.T) {
	var calls atomic.Int32
	s, _ := launchServerSampling(t, SamplerConfig{Interval: 5 * time.Millisecond, ReadGPUs: func(context.Context) ([]GPUReading, error) {
		calls.Add(1)
		return nil, ErrNoNvidiaSmi
	}})
	_ = s
	time.Sleep(150 * time.Millisecond) // about thirty ticks
	if n := calls.Load(); n != 0 {
		t.Errorf("nvidia-smi was read %d times with no active run", n)
	}
}

func TestSampler_TheLoopDoesTickWhenARunIsActive(t *testing.T) {
	var calls atomic.Int32
	s, fe := launchServerSampling(t, SamplerConfig{Interval: 5 * time.Millisecond, ReadGPUs: func(context.Context) ([]GPUReading, error) {
		calls.Add(1)
		return nil, ErrNoNvidiaSmi
	}})
	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0"})
	waitStart(t, fe)
	waitFor(t, "ticks while active", func() bool { return calls.Load() >= 5 })
	fe.chanFor(id) <- true
	waitState(t, s, id, StateSucceeded)
}

func TestSampler_ARunIsSampledWhileActiveAndNotAfter(t *testing.T) {
	s, fe := launchServerSampling(t, SamplerConfig{Interval: 10 * time.Millisecond, ReadGPUs: twoGPUs(1000, 9000), ReadRAM: fixedRAM(5000, 30000)})

	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "1"})
	waitStart(t, fe)
	waitFor(t, "several samples", func() bool { return len(samplesOf(t, s, id)) >= 4 })
	for _, r := range samplesOf(t, s, id) {
		if r.GPU != 1 || r.VRAMUsedMB != 9000 {
			t.Fatalf("a run on GPU 1 stores only GPU 1: %+v", r)
		}
	}
	fe.chanFor(id) <- true
	waitState(t, s, id, StateSucceeded)
	time.Sleep(60 * time.Millisecond) // let a late tick land
	settled := len(samplesOf(t, s, id))
	time.Sleep(120 * time.Millisecond)
	if n := len(samplesOf(t, s, id)); n != settled {
		t.Errorf("sampling must stop with the run: %d then %d rows", settled, n)
	}
}

func TestSampler_AReferenceSampleIsTakenWhenTheRunStarts(t *testing.T) {
	s, fe := launchServerSampling(t, SamplerConfig{Interval: time.Hour, ReadGPUs: twoGPUs(500, 700), ReadRAM: fixedRAM(3000, 30000)}) // the loop will not tick: only the start sample can exist
	id, _ := launchRun(t, s, "demo", map[string]any{"gpu": "0"})
	waitStart(t, fe)
	waitFor(t, "the reference sample", func() bool { return len(samplesOf(t, s, id)) == 1 })
	if r := samplesOf(t, s, id)[0]; r.GPU != 0 || r.VRAMUsedMB != 500 {
		t.Errorf("reference sample: %+v", r)
	}
	fe.chanFor(id) <- true
	waitState(t, s, id, StateSucceeded)
}

func TestResources_PeaksSeriesAndLabelsOfARun(t *testing.T) {
	s, _ := launchServer(t)
	start := d0
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateSucceeded, created: start, durS: 10})
	add := func(off time.Duration, gpu, used, total, util, ram int) {
		row := RunSample{RunID: "r", TS: start.Add(off), GPU: gpu, VRAMUsedMB: used, VRAMTotalMB: total, UtilPct: util, RAMUsedMB: ram, RAMTotalMB: 30000}
		if err := s.rm.hist.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	add(0, 1, 1000, 16311, 0, 5000)
	add(2*time.Second, 1, 14200, 16311, 95, 21000)
	add(2*time.Second, 0, 800, 12282, 1, 21000)
	add(4*time.Second, 1, 9000, 16311, 40, 12000)

	code, res, body := getResources(t, s, "r")
	if code != 200 || res.IntervalMs != 2000 || res.Truncated {
		t.Fatalf("%d %s", code, body)
	}
	if len(res.GPUs) != 2 || res.GPUs[0].Index != 0 || res.GPUs[1].Index != 1 || res.GPUs[1].TotalMB != 16311 || len(res.GPUs[1].Samples) != 3 {
		t.Fatalf("gpus: %+v", res.GPUs)
	}
	if p := res.GPUs[1].Samples[1]; p.TMs != 2000 || p.UsedMB != 14200 || p.UtilPct == nil || *p.UtilPct != 95 {
		t.Errorf("sample: %+v", p)
	}
	var peak1 vramPeak
	for _, p := range res.Peaks.VRAM {
		if p.GPU == 1 {
			peak1 = p
		}
	}
	if peak1.MB != 14200 || !near(peak1.Pct, 87.1) {
		t.Errorf("VRAM peak of GPU 1 = %+v, want 14200 MB / 87.1 %%", peak1)
	}
	if res.RAM.TotalMB != 30000 || len(res.RAM.Samples) != 3 {
		t.Errorf("one RAM point per instant, not per row: %+v", res.RAM)
	}
	if res.Peaks.RAM == nil || res.Peaks.RAM.MB != 21000 || !near(res.Peaks.RAM.Pct, 70) {
		t.Errorf("RAM peak: %+v", res.Peaks.RAM)
	}
}

func TestResources_ARunWithoutSamplesAnswersEmptyListsNotAnError(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "old", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	code, res, body := getResources(t, s, "old")
	if code != 200 || len(res.GPUs) != 0 || len(res.RAM.Samples) != 0 || res.Peaks.RAM != nil || len(res.Peaks.VRAM) != 0 {
		t.Fatalf("%d %s", code, body)
	}
	for _, f := range []string{`"gpus":[]`, `"samples":[]`, `"vram":[]`, `"ram":null`} {
		if !strings.Contains(body, f) {
			t.Errorf("must serialize as %s: %s", f, body)
		}
	}
}

func TestResources_AnUnknownRunIs404(t *testing.T) {
	s, _ := launchServer(t)
	if code, _, _ := getResources(t, s, "nope"); code != http.StatusNotFound {
		t.Errorf("%d, want 404", code)
	}
}

func TestResources_RAMOnlyRowsGiveNoGPUSeries(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "r", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	_ = s.rm.hist.db.Create(&RunSample{RunID: "r", TS: d0, GPU: -1, RAMUsedMB: 4000, RAMTotalMB: 16000}).Error
	_, res, _ := getResources(t, s, "r")
	if len(res.GPUs) != 0 || len(res.RAM.Samples) != 1 || res.Peaks.RAM == nil || !near(res.Peaks.RAM.Pct, 25) {
		t.Errorf("%+v", res)
	}
}

func TestStats_PerGPUPeaksComeFromTheSamples(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, gpu: "1", created: d0, durS: 5})
	seedStat(t, s, statRun{id: "b", project: "demo", state: StateSucceeded, gpu: "1", created: d0, durS: 5})
	seedStat(t, s, statRun{id: "c", project: "demo", state: StateSucceeded, gpu: "0", created: d0, durS: 5}) // no samples
	for id, used := range map[string]int{"a": 8000, "b": 14000} {
		_ = s.rm.hist.db.Create(&RunSample{RunID: id, TS: d0, GPU: 1, VRAMUsedMB: used, VRAMTotalMB: 16000}).Error
	}
	res, _ := getStats(t, s, wide)
	g := map[string]statsGroup{}
	for _, x := range res.ByGPU {
		g[x.Name] = x
	}
	if g["1"].PeakVRAMPct == nil || !near(*g["1"].PeakVRAMPct, 87.5) || g["1"].MeanPeakVRAMPct == nil || !near(*g["1"].MeanPeakVRAMPct, 68.8) || g["1"].SampledRuns != 2 {
		t.Errorf("GPU 1: %+v", g["1"])
	}
	if g["0"].PeakVRAMPct != nil {
		t.Errorf("a GPU without samples has no peaks: %+v", g["0"])
	}
}

func TestRunDelete_TakesTheResourceSamplesWithTheRun(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 5, events: []string{ev("phase_end", "render", 1)}})
	seedStat(t, s, statRun{id: "b", project: "demo", state: StateSucceeded, created: d0, durS: 5})
	for _, id := range []string{"a", "b"} {
		_ = s.rm.hist.db.Create(&RunSample{RunID: id, TS: d0, GPU: 1, VRAMUsedMB: 1, VRAMTotalMB: 2}).Error
	}
	if rec := deleteRun(s, "a"); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", rec.Code)
	}
	if len(samplesOf(t, s, "a")) != 0 || len(samplesOf(t, s, "b")) != 1 {
		t.Error("the samples of a deleted run go with it, the others stay")
	}
}

func TestRunDelete_AFailureLeavesTheSamplesToo(t *testing.T) {
	s, _ := launchServer(t)
	seedStat(t, s, statRun{id: "a", project: "demo", state: StateSucceeded, created: d0, durS: 5, events: []string{ev("phase_end", "render", 1)}})
	_ = s.rm.hist.db.Create(&RunSample{RunID: "a", TS: d0, GPU: 1, VRAMUsedMB: 1, VRAMTotalMB: 2}).Error
	s.rm.hist.afterEventsDeleted = func() error { return errors.New("disk full") }
	if rec := deleteRun(s, "a"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("DELETE: %d", rec.Code)
	}
	if !runExists(s, "a") || eventCount(t, s, "a") != 1 || len(samplesOf(t, s, "a")) != 1 {
		t.Error("run, events and samples must all survive a failed deletion")
	}
}

func TestParsers_NvidiaSMIAndMeminfo(t *testing.T) {
	got, err := parseGPUReadings("0, 1210, 12282, 3\n1, 27, 16311, 0\n\n")
	if err != nil || len(got) != 2 || got[0] != (GPUReading{0, 1210, 12282, 3}) || got[1].TotalMB != 16311 {
		t.Errorf("parseGPUReadings = %+v, %v", got, err)
	}
	got, err = parseGPUReadings("0, 100, 200, [N/A]\n")
	if err != nil || got[0].UtilPct != 0 {
		t.Errorf("an unavailable counter reads as 0: %+v %v", got, err)
	}
	for _, bad := range []string{"0, 1, 2", "0, x, 2, 3"} {
		if _, err := parseGPUReadings(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	used, total, err := parseMeminfo("MemTotal:       32768000 kB\nMemFree:  100 kB\nMemAvailable:   10240000 kB\n")
	if err != nil || total != 32000 || used != 22000 {
		t.Errorf("parseMeminfo = %d/%d %v", used, total, err)
	}
	if _, _, err := parseMeminfo("MemTotal: 1 kB\n"); err == nil {
		t.Error("missing MemAvailable must be an error")
	}
}
