//go:build !private

package server

import (
	"path/filepath"
	"testing"
)

// The public build cannot run in the mode that hides nothing, however it is asked.
func TestContentMode_ThePublicBuildRefusesToShowEverything(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(Config{ProjectsDir: filepath.Join(dir, "p"), DBPath: filepath.Join(dir, "o.db"), ContentMode: ContentAll}); err == nil {
		t.Fatal("New must refuse the mode")
	}
}
