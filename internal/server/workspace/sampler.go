package workspace

// sampler.go records the resources a run uses while it is active: for every
// GPU the run occupies, the VRAM and utilization of that GPU, plus the
// machine's RAM, every couple of seconds (see specs/run-statistics). The VRAM
// is that of the whole GPU at that instant, not of the run's processes.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultSampleInterval is the time between samples of an active run.
const defaultSampleInterval = 2 * time.Second

// maxSampleRows bounds what one run stores; a longer run stops sampling.
const maxSampleRows = 20000

// RunSample is one reading for one run. GPU is the nvidia-smi index, or -1
// for a RAM-only row (no nvidia-smi, or no GPU reported).
type RunSample struct {
	ID          uint      `gorm:"primaryKey" json:"-"`
	RunID       string    `gorm:"size:64;index" json:"run_id"`
	TS          time.Time `json:"ts"`
	GPU         int       `json:"gpu"`
	VRAMUsedMB  int       `gorm:"column:vram_used_mb" json:"vram_used_mb"`
	VRAMTotalMB int       `gorm:"column:vram_total_mb" json:"vram_total_mb"`
	UtilPct     int       `json:"util_pct"`
	RAMUsedMB   int       `json:"ram_used_mb"`
	RAMTotalMB  int       `json:"ram_total_mb"`
}

// GPUReading is one card at one moment: its index, the memory in use and in all, and how busy it is.
type GPUReading struct {
	Index, UsedMB, TotalMB, UtilPct int
}

// ErrNoNvidiaSmi means the GPU cannot be read at all (as opposed to a
// transient failure): only the RAM is then sampled.
var ErrNoNvidiaSmi = errors.New("nvidia-smi is not available")

// SamplerConfig is how the resources of an active run are read. A zero field is the real thing: every 2 seconds, the
// cards through nvidia-smi and the memory through /proc/meminfo. A test gives its own so no real hardware is read.
type SamplerConfig struct {
	Interval time.Duration
	ReadGPUs func(ctx context.Context) ([]GPUReading, error)
	ReadRAM  func() (usedMB, totalMB int, err error)
}

// NoHardware is a SamplerConfig that sees no card and a fixed memory.
func NoHardware() SamplerConfig {
	return SamplerConfig{
		ReadGPUs: func(context.Context) ([]GPUReading, error) { return nil, ErrNoNvidiaSmi },
		ReadRAM:  func() (int, int, error) { return 4096, 16384, nil },
	}
}

type resourceSampler struct {
	hist     *historyStore
	readGPUs func(ctx context.Context) ([]GPUReading, error)
	readRAM  func() (usedMB, totalMB int, err error)

	mu       sync.Mutex
	interval time.Duration
	maxRows  int
	rows     map[string]int // rows stored per active run
}

func newResourceSampler(h *historyStore, c SamplerConfig) *resourceSampler {
	if c.Interval == 0 {
		c.Interval = defaultSampleInterval
	}
	if c.ReadGPUs == nil {
		c.ReadGPUs = readGPUsNvidiaSMI
	}
	if c.ReadRAM == nil {
		c.ReadRAM = readRAMProc
	}
	return &resourceSampler{
		hist:     h,
		readGPUs: c.ReadGPUs,
		readRAM:  c.ReadRAM,
		interval: c.Interval,
		maxRows:  maxSampleRows,
		rows:     make(map[string]int),
	}
}

func (s *resourceSampler) intervalNow() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// forget drops the bookkeeping of a run that ended.
func (s *resourceSampler) forget(runID string) {
	s.mu.Lock()
	delete(s.rows, runID)
	s.mu.Unlock()
}

// sampleTarget is a run to sample and the GPUs it occupies; a run that names
// none occupies every GPU the reader reports.
type sampleTarget struct {
	runID string
	gpus  []int
}

func targetOf(runID string, lanes []string) sampleTarget {
	t := sampleTarget{runID: runID}
	for _, l := range lanes {
		if n, err := strconv.Atoi(l); err == nil {
			t.gpus = append(t.gpus, n)
		}
	}
	return t
}

func (t sampleTarget) occupies(index int) bool {
	if len(t.gpus) == 0 {
		return true
	}
	for _, g := range t.gpus {
		if g == index {
			return true
		}
	}
	return false
}

// sample takes one reading and stores it for every target. A failed GPU read
// skips the sample; it never affects a run.
func (s *resourceSampler) sample(ctx context.Context, targets []sampleTarget) {
	if len(targets) == 0 {
		return
	}
	gpus, err := s.readGPUs(ctx)
	noGPU := errors.Is(err, ErrNoNvidiaSmi)
	if err != nil && !noGPU {
		return
	}
	ramUsed, ramTotal, rerr := s.readRAM()
	if rerr != nil {
		ramUsed, ramTotal = 0, 0 // unknown: stored as zero, the screens hide it
	}
	now := time.Now().UTC()
	for _, t := range targets {
		var rows []RunSample
		if !noGPU {
			for _, g := range gpus {
				if t.occupies(g.Index) {
					rows = append(rows, RunSample{RunID: t.runID, TS: now, GPU: g.Index,
						VRAMUsedMB: g.UsedMB, VRAMTotalMB: g.TotalMB, UtilPct: g.UtilPct,
						RAMUsedMB: ramUsed, RAMTotalMB: ramTotal})
				}
			}
		}
		if len(rows) == 0 {
			rows = []RunSample{{RunID: t.runID, TS: now, GPU: -1, RAMUsedMB: ramUsed, RAMTotalMB: ramTotal}}
		}
		s.mu.Lock()
		over := s.rows[t.runID]+len(rows) > s.maxRows
		if !over {
			s.rows[t.runID] += len(rows)
		}
		s.mu.Unlock()
		if over {
			continue // truncated: this run stops being sampled
		}
		if err := s.hist.db.Create(&rows).Error; err != nil {
			slog.Warn("cannot store the samples of a run", "run", t.runID, "err", err)
		}
	}
}

// --- real readers -----------------------------------------------------------

func readGPUsNvidiaSMI(ctx context.Context) ([]GPUReading, error) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, ErrNoNvidiaSmi
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path,
		"--query-gpu=index,memory.used,memory.total,utilization.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi failed: %w", err)
	}
	return parseGPUReadings(string(out))
}

// parseGPUReadings parses "index, used, total, util" lines (nounits csv).
func parseGPUReadings(out string) ([]GPUReading, error) {
	var res []GPUReading
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 4 {
			return nil, fmt.Errorf("unexpected nvidia-smi line %q", line)
		}
		var n [4]int
		for i := 0; i < 4; i++ {
			v := strings.TrimSpace(f[i])
			if v == "[N/A]" || v == "[Not Supported]" {
				continue // an unavailable counter reads as 0
			}
			x, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("unexpected nvidia-smi value %q", v)
			}
			n[i] = x
		}
		res = append(res, GPUReading{Index: n[0], UsedMB: n[1], TotalMB: n[2], UtilPct: n[3]})
	}
	return res, sc.Err()
}

// readRAMProc returns the machine's used and total RAM in MB, used being
// MemTotal minus MemAvailable.
func readRAMProc() (int, int, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	return parseMeminfo(string(b))
}

func parseMeminfo(s string) (int, int, error) {
	var total, avail int64 = -1, -1
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	if total < 0 || avail < 0 {
		return 0, 0, errors.New("MemTotal or MemAvailable missing")
	}
	return int((total - avail) / 1024), int(total / 1024), nil
}
