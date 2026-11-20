package workspace

import (
	"os"
	"testing"
)

// TestMain keeps the whole suite independent of the developer's own keys: a Hugging Face or Civitai key saved on this
// machine must not change what a test sees, and no test reads the real hardware.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "workspace-test-home-")
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
