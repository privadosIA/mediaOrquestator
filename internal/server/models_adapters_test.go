package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdapters_EnvironmentReportsThem(t *testing.T) {
	s, _ := newProjectsServer(t)
	dir := filepath.Join("models", "music", "adapters", "style")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "adapter_model.safetensors"), make([]byte, 40), 0o644)
	os.WriteFile(filepath.Join(dir, "adapter_config.json"), make([]byte, 2), 0o644)
	rep := getEnvironment(t, s, "")
	if len(rep.MusicAdapters) != 1 || rep.MusicAdapters[0].Name != "style" || rep.MusicAdapters[0].Size != 42 {
		t.Fatalf("music_adapters = %+v", rep.MusicAdapters)
	}
	if len(rep.Music) != 0 {
		t.Errorf("adapters must not be listed among the music models: %+v", rep.Music)
	}
}
