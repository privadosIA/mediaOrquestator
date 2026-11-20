package sdlog

import (
	"os"
	"testing"
)

func TestLoadMillis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		out    string
		wantMS int
		wantOK bool
	}{
		{"one load", "[I] loading tensors completed, taking 0.20s (read: 0.04s) --- model_loader.cpp:1381\n", 200, true},
		{"several components add up", "[I] loading tensors completed, taking 0.20s (x)\n[I] loading tensors completed, taking 1.20s (x)\n", 1400, true},
		{"milliseconds", "[I] loading tensors completed, taking 350 ms (x)\n", 350, true},
		{"mixed units", "loading tensors completed, taking 1.5s\nloading tensors completed, taking 250 ms\n", 1750, true},
		{"rounded to the millisecond", "loading tensors completed, taking 0.0005s\n", 1, true},
		{"no load lines", "[I] sampling completed, taking 4.34s --- image.cpp:899\n[I] generate_image completed in 6.06s\n", 0, false},
		{"other timings are not loads", "[I] get_learned_condition completed, taking 0.98s\n[I] encode_first_stage completed, taking 721 ms\n", 0, false},
		{"empty", "", 0, false},
		{"nil-like noise", "\x1b[K |####| 3/178\r garbage taking 3s", 0, false},
		{"a malformed number is skipped", "loading tensors completed, taking 1.2.3s\nloading tensors completed, taking 0.5s\n", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms, ok := LoadMillis([]byte(tc.out))
			if ms != tc.wantMS || ok != tc.wantOK {
				t.Errorf("LoadMillis = %d, %v; want %d, %v", ms, ok, tc.wantMS, tc.wantOK)
			}
		})
	}
}

// The outputs saved from the pinned sd (RTX 5060 Ti, 2026-11-02): an image and a clip.
func TestLoadMillisOnRealOutputs(t *testing.T) {
	for file, want := range map[string]int{
		"testdata/render-sdxl.out": 2200, // 0.20 + 0.40 + 0.20 + 1.20 + 0.20 of the 6.06 s of generate_image
		"testdata/clip-wan5b.out":  1930, // 0.20 + 0.72 + 0.61 + 0.40 of the 26.33 s of generate_video
	} {
		out, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if ms, ok := LoadMillis(out); !ok || ms != want {
			t.Errorf("%s: LoadMillis = %d, %v; want %d, true", file, ms, ok, want)
		}
	}
}
