package clap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissing_ASnapshotWithConfigAndWeightsIsComplete(t *testing.T) {
	dir := t.TempDir()
	if got := Missing(dir); len(got) != 1 || got[0] != model {
		t.Fatalf("empty folder: %v", got)
	}
	snap := filepath.Join(dir, "hub", model, "snapshots", "abc")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(snap, "config.json"), []byte("{}"), 0o644)
	if got := Missing(dir); len(got) != 1 {
		t.Fatalf("a snapshot without weights is incomplete: %v", got)
	}
	os.WriteFile(filepath.Join(snap, "pytorch_model.bin"), []byte("x"), 0o644)
	if got := Missing(dir); got != nil {
		t.Fatalf("complete snapshot: %v", got)
	}
}
