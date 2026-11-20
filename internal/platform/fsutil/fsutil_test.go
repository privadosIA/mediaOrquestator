package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "new", "deep", "b.bin")
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "data" {
		t.Fatalf("copy: %q %v", b, err)
	}
	if fi, _ := os.Stat(dst); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want the source's 0600", fi.Mode().Perm())
	}
	ents, _ := os.ReadDir(filepath.Dir(dst))
	if len(ents) != 1 {
		t.Errorf("the folder holds %d entries, want only the copy", len(ents))
	}
	if err := CopyFile(filepath.Join(dir, "missing"), dst); err == nil {
		t.Error("a missing source must fail")
	}
	if b, _ := os.ReadFile(dst); string(b) != "data" {
		t.Error("a failed copy must leave the old file alone")
	}
}

func TestExistsAndMedian(t *testing.T) {
	if !Exists(t.TempDir()) || Exists("/nonexistent/x") {
		t.Error("Exists")
	}
	for _, c := range []struct {
		in   []float64
		want float64
	}{{nil, 0}, {[]float64{5}, 5}, {[]float64{3, 1, 2}, 2}, {[]float64{4, 1, 3, 2}, 2.5}} {
		in := append([]float64(nil), c.in...)
		if got := Median(in); got != c.want {
			t.Errorf("Median(%v) = %v, want %v", c.in, got, c.want)
		}
		for i := range in {
			if in[i] != c.in[i] {
				t.Errorf("Median changed its input")
			}
		}
	}
}
