package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// webBuild writes a small front-end build and returns its directory.
func webBuild(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"index.html":           "<html>shell</html>",
		"chunk-0i58r8Jb.js":    "console.log(1)",
		"main.js":              "console.log(2)",
		"ngsw.json":            "{}",
		"ngsw-worker.js":       "//worker",
		"manifest.webmanifest": "{}",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func webServer(t *testing.T, dir string) http.Handler {
	t.Helper()
	s, err := New(Config{Addr: "127.0.0.1:0", ProjectsDir: t.TempDir(), DBPath: t.TempDir() + "/o.db", WebDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

func fetch(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestWebDir_ServesABuild(t *testing.T) {
	h := webServer(t, webBuild(t))
	if res, body := fetch(t, h, "/"); res.StatusCode != 200 || body != "<html>shell</html>" {
		t.Fatalf("/: %d %q", res.StatusCode, body)
	}
	if res, body := fetch(t, h, "/main.js"); res.StatusCode != 200 || body != "console.log(2)" {
		t.Fatalf("/main.js: %d %q", res.StatusCode, body)
	}
}

func TestWebDir_RouteOfTheFrontEndGetsTheShell(t *testing.T) {
	h := webServer(t, webBuild(t))
	for _, p := range []string{"/runs/12", "/create", "/chunk-ZZZZZZZZ.js"} {
		res, body := fetch(t, h, p)
		if res.StatusCode != 200 || body != "<html>shell</html>" {
			t.Errorf("%s: %d %q", p, res.StatusCode, body)
		}
		if cc := res.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: the shell served for a missing file must not be cached for good, got %q", p, cc)
		}
	}
}

func TestWebDir_UnknownAPIPathStaysJSON404(t *testing.T) {
	h := webServer(t, webBuild(t))
	res, body := fetch(t, h, "/api/nope")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", res.StatusCode)
	}
	decodeAPIError(t, body)
}

func TestWebDir_CachingByName(t *testing.T) {
	h := webServer(t, webBuild(t))
	if cc := func() string { r, _ := fetch(t, h, "/chunk-0i58r8Jb.js"); return r.Header.Get("Cache-Control") }(); cc != "public, max-age=31536000, immutable" {
		t.Errorf("hashed file: %q", cc)
	}
	for _, p := range []string{"/", "/index.html", "/ngsw.json", "/ngsw-worker.js", "/manifest.webmanifest", "/main.js"} {
		res, _ := fetch(t, h, p)
		if res.StatusCode == http.StatusOK && res.Header.Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", p, res.Header.Get("Cache-Control"))
		}
	}
	if hashedName.MatchString("ngsw-worker.js") {
		t.Error("the worker has a fixed name and must be revalidated")
	}
}

func TestWebDir_ManifestType(t *testing.T) {
	res, _ := fetch(t, webServer(t, webBuild(t)), "/manifest.webmanifest")
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestWebDir_NewBuildWithoutRestart(t *testing.T) {
	dir := webBuild(t)
	h := webServer(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>v2</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, body := fetch(t, h, "/"); body != "<html>v2</html>" {
		t.Fatalf("body = %q", body)
	}
}

func TestWebDir_CannotLeaveTheDirectory(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "site")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := webDirHandler(dir)
	// URL.Path is what the server sees after decoding, so these are the decoded forms of ../ and %2e%2e/.
	for _, p := range []string{"/../secret.txt", "/a/../../secret.txt", "/..//secret.txt", "/./../secret.txt", "../secret.txt"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = p
		h.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "SECRET") {
			t.Errorf("%s returned a file outside the directory", p)
		}
	}
	// Control: the same name inside the directory is served, so the test would notice a leak.
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("INSIDE"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = "/a/../../secret.txt"
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "INSIDE" {
		t.Errorf("a cleaned path resolves inside the directory, got %q", rec.Body.String())
	}
}

func TestWebDir_NoDirectoryGivesTheNote(t *testing.T) {
	h := newTestServer(t).Handler()
	res, body := fetch(t, h, "/")
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") || !strings.Contains(body, "/api/") || !strings.Contains(body, "--web-dir") {
		t.Fatalf("%s %q", res.Header.Get("Content-Type"), body)
	}
	if res, _ := fetch(t, h, "/api/health"); res.StatusCode != 200 {
		t.Fatalf("API health: %d", res.StatusCode)
	}
}
