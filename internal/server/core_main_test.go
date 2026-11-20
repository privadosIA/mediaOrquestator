package server

import (
	"os"
	"testing"
)

// TestMain keeps the whole suite independent of the developer's own keys: a
// Hugging Face or Civitai key saved on this machine (in the environment or in
// the key files under $HOME) must not change what a test sees. Tests that need
// a key set their own HOME and variables with t.Setenv.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "server-test-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	for _, v := range []string{"HF_TOKEN", "CIVITAI_TOKEN", "HF_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
		_ = os.Unsetenv(v)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
