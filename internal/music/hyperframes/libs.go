// Package hyperframes keeps whole HyperFrames blocks and the local copies of the libraries they load from a CDN, so that
// drawing a block never touches the network. The files of a block are in its folder under the store's Dir (its page loads them by relative path); its
// record (title, tags, the result of the checks...) is a row of the database (NewStoreDB, db.go) or, without one, its block.json.
package hyperframes

import (
	_ "embed"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

//go:embed libs.txt
var libsList string

// LibsDir is where scripts/get-hyperframes-libs.sh puts the libraries, relative to the server's working directory.
var LibsDir = filepath.Join("third_party", "hyperframes-libs")

// Lib is one file a block loads from outside: its address, the SHA-256 it is pinned to and where it is kept.
type Lib struct {
	URL    string
	SHA256 string
}

// Manifest lists the pinned libraries.
func Manifest() []Lib {
	var out []Lib
	for _, line := range strings.Split(libsList, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		out = append(out, Lib{SHA256: f[0], URL: f[1]})
	}
	return out
}

// File is where the library is kept under dir: <dir>/<host>/<path>. A URL that is not https, has no path or tries to leave
// dir has no file.
func File(dir, rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path == "" || strings.HasSuffix(u.Path, "/") {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(u.Path))
	if strings.HasPrefix(clean, "..") {
		return "", false
	}
	return filepath.Join(dir, u.Host, clean), true
}

// Lookup is the local copy of the library a page asks for (the query and the fragment do not count), when it is pinned and
// installed. Anything else, installed or not, is not served.
func Lookup(dir, rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	u.RawQuery, u.Fragment = "", ""
	clean := u.String()
	for _, l := range Manifest() {
		if l.URL != clean {
			continue
		}
		p, ok := File(dir, clean)
		if !ok {
			return "", false
		}
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			return "", false
		}
		return p, true
	}
	return "", false
}

// Missing lists the pinned libraries that are not installed under dir.
func Missing(dir string) []Lib {
	var out []Lib
	for _, l := range Manifest() {
		p, ok := File(dir, l.URL)
		if !ok {
			continue
		}
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			out = append(out, l)
		}
	}
	return out
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }
