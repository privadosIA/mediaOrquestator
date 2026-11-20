// Package motion renders animated HTML pages to video with a headless Chrome or
// Chromium, frame by frame, and holds what writes and checks those pages. It is
// the source of the motion-graphics scenes and the transparent overlays.
package motion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNoBrowser means neither the configured path nor the PATH holds Chrome or Chromium.
var ErrNoBrowser = errors.New("no Chrome or Chromium found")

// Names are the executables looked up in the PATH, in order.
var Names = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"}

// InstallHint tells a user without a browser what to do.
const InstallHint = "install Google Chrome or Chromium with your system package manager, or set its path in Settings"

// Browser is a Chrome or Chromium found on this machine.
type Browser struct {
	Path    string
	Version string
}

// Find returns the browser to use: the file at configured when it is set (an
// invalid path is an error, not a silent fallback), else the first of Names in
// the PATH. The version is asked from the browser, best effort.
func Find(configured string) (Browser, error) {
	if p := strings.TrimSpace(configured); p != "" {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return Browser{}, fmt.Errorf("the browser path %q is not an executable file", p)
		}
		return Browser{Path: p, Version: versionOf(p)}, nil
	}
	for _, n := range Names {
		if p, err := exec.LookPath(n); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			return Browser{Path: p, Version: versionOf(p)}, nil
		}
	}
	return Browser{}, fmt.Errorf("%w: %s", ErrNoBrowser, InstallHint)
}

var versionRe = regexp.MustCompile(`\d+(\.\d+){1,3}`)

// versionOf asks the browser for its version: the number, or "" when it will not say.
func versionOf(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return versionRe.FindString(string(out))
}
