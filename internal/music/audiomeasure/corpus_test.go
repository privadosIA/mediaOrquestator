package audiomeasure

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCorpus measures real songs whose tempo and key were requested when they
// were generated. It runs only with MEASURE_CORPUS=<file>, a text file with one
// line per song: path<TAB>bpm<TAB>key. It prints the table the documentation
// quotes (see openspec/changes/extract-song-from-media).
func TestCorpus(t *testing.T) {
	path := os.Getenv("MEASURE_CORPUS")
	if path == "" {
		t.Skip("MEASURE_CORPUS not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var n, bpmOK, bpmExact, keyExact, keyRel int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) < 3 {
			continue
		}
		want, _ := strconv.ParseFloat(cols[1], 64)
		out, err := exec.Command("ffmpeg", "-v", "error", "-i", cols[0], "-vn", "-ac", "1", "-ar", strconv.Itoa(Rate), "-f", "f32le", "-").Output()
		if err != nil {
			t.Fatalf("%s: %v", cols[0], err)
		}
		s := make([]float32, len(out)/4)
		for i := range s {
			s[i] = math.Float32frombits(binary.LittleEndian.Uint32(out[i*4:]))
		}
		start := time.Now()
		r := Measure(s)
		took := time.Since(start)
		gotBPM, gotKey := "-", "-"
		hit := false
		if r.BPM != nil {
			gotBPM = fmt.Sprintf("%.1f", *r.BPM)
			for _, c := range append([]float64{*r.BPM}, r.BPMAlternative...) {
				for _, mult := range []float64{1, 0.5, 2} {
					if math.Abs(c-want*mult)/(want*mult) < 0.04 {
						hit = true
					}
				}
			}
			if math.Abs(*r.BPM-want)/want < 0.04 {
				bpmExact++
			}
		}
		if hit {
			bpmOK++
		}
		if r.Key != nil {
			gotKey = *r.Key
			if *r.Key == cols[2] {
				keyExact++
			} else if relative(*r.Key, cols[2]) {
				keyRel++
			}
		}
		n++
		fmt.Printf("%-55s want %5.0f %-9s got %6s (alt %v conf %.2f) %-9s conf %.2f  %v\n", cols[0], want, cols[2], gotBPM, r.BPMAlternative, r.BPMConfidence, gotKey, r.KeyConfidence, took.Round(time.Millisecond))
	}
	fmt.Printf("songs %d | bpm direct %d, with half/double %d | key exact %d, relative %d\n", n, bpmExact, bpmOK, keyExact, keyRel)
}

// relative reports whether two keys share their notes (C major / A minor).
func relative(a, b string) bool {
	pa, pb := strings.Fields(a), strings.Fields(b)
	if len(pa) != 2 || len(pb) != 2 || pa[1] == pb[1] {
		return false
	}
	idx := func(n string) int {
		for i, v := range noteNames {
			if v == n {
				return i
			}
		}
		return -1
	}
	ma, mi := pa, pb
	if pa[1] == "minor" {
		ma, mi = pb, pa
	}
	return (idx(mi[0])+3)%12 == idx(ma[0])
}
