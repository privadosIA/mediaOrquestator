package edition

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests look at the source tree, not at behaviour. They run in both
// editions and in the exported public tree (scripts/sync-public.sh runs them
// there before it commits): the public tree must not hold private code.

const privateList = "scripts/private-files.txt"

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "third_party": true, "dist": true, ".angular": true, "test-results": true, "bin": true, "projects": true, "models": true}

// walk calls fn for every file of the tree that is source (not dependencies or build output).
func walk(t *testing.T, root string, fn func(rel string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		fn(filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func firstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			return l
		}
	}
	return ""
}

func readList(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(root, privateList))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out[l] = true
		}
	}
	return out
}

// privateSet is what the tree says is private: the list (private tree) and every
// Go file tagged private.
func privateSet(t *testing.T, root string) (tagged []string, list map[string]bool) {
	t.Helper()
	walk(t, root, func(rel string) {
		if strings.HasSuffix(rel, ".go") && firstLine(filepath.Join(root, rel)) == "//go:build private" {
			tagged = append(tagged, rel)
		}
	})
	if exists(root, privateList) {
		list = readList(t, root)
	}
	return tagged, list
}

func TestPublicTreeHoldsNoPrivateCode(t *testing.T) {
	root := repoRoot(t)
	tagged, list := privateSet(t, root)
	privateTree := exists(root, "internal/platform/edition/edition_private.go")
	if !privateTree {
		for _, f := range tagged {
			t.Errorf("the public tree holds a private file: %s", f)
		}
		if list != nil {
			t.Errorf("the public tree holds %s", privateList)
		}
		return
	}
	if list == nil {
		t.Fatalf("the private tree needs %s", privateList)
	}
	for _, f := range tagged {
		if !list[f] {
			t.Errorf("%s is tagged private but is not in %s", f, privateList)
		}
	}
	for f := range list {
		if !exists(root, f) {
			t.Errorf("%s lists %s, which does not exist", privateList, f)
		}
	}
}

// modelWords name models without safety training: the words of the deny list, as whole words, and the run "muse glimmer".
// They may appear only in the deny list itself, in the tests of the deny list and in the private files; a specification,
// a document or any other test that needs a model name uses an invented one.
var modelWords = regexp.MustCompile(`(?i)(^|[^a-z])(abliterated|uncensored|unfiltered|dolphin|lexi)([^a-z]|$)|muse[-_ .:]glimmer`)

// denyListTestFiles are tests, besides the ones with "edition" in their name, whose job includes trying the deny list with
// real names (the describe test checks that a denied model is not listed or picked in the public edition).
var denyListTestFiles = map[string]bool{
	"internal/platform/edition/tree_test.go":       true,
	"internal/assistant/describe/describe_test.go": true,
}

// denyListTest says whether a file may name the models of the deny list.
func denyListTest(rel string) bool {
	base := filepath.Base(rel)
	return strings.HasSuffix(base, "_test.go") && (strings.Contains(base, "edition") || denyListTestFiles[rel])
}

func TestModelWordsOnlyInTheDenyList(t *testing.T) {
	root := repoRoot(t)
	_, list := privateSet(t, root)
	tagged, _ := privateSet(t, root)
	private := map[string]bool{}
	for f := range list {
		private[f] = true
	}
	for _, f := range tagged {
		private[f] = true
	}
	public := !exists(root, "internal/platform/edition/edition_private.go")
	walk(t, root, func(rel string) {
		if private[rel] || rel == "internal/platform/edition/edition_public.go" || denyListTest(rel) ||
			strings.HasPrefix(rel, "openspec/changes/archive/") || strings.HasPrefix(rel, "openspec/specs/editions/") {
			return
		}
		ext := filepath.Ext(rel)
		switch {
		case ext == ".go" || ext == ".ts" || ext == ".html" || ext == ".json" || ext == ".yaml" || ext == ".sh":
		case public && ext == ".md":
		default:
			return
		}
		if strings.HasPrefix(rel, "web/src/locale/") {
			return
		}
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return
		}
		if m := modelWords.Find(b); m != nil {
			t.Errorf("%s names %q outside the deny list", rel, strings.TrimSpace(string(m)))
		}
	})
}

func TestBaseCatalogHasNoCivitaiEntry(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal/server/catalog.json"))
	if err != nil {
		t.Skip("no server catalog in this tree")
	}
	if strings.Contains(string(b), `"requires_token": "civitai"`) {
		t.Error("internal/server/catalog.json holds an entry that needs a Civitai key: it belongs in catalog_private.json")
	}
}

// The age check is the same in both editions: nothing in it may depend on the edition.
func TestAgeCheckHasNoEditionBranch(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal/assistant/describe/describe.go"))
	if err != nil {
		t.Skip("no describe package in this tree")
	}
	src := string(b)
	i := strings.Index(src, "func (d *Describer) checkAdults(")
	if i < 0 {
		t.Fatal("checkAdults is gone: update this test")
	}
	body := src[i:]
	if j := strings.Index(body[1:], "\nfunc "); j >= 0 {
		body = body[:j+1]
	}
	if strings.Contains(body, "edition.") {
		t.Error("checkAdults depends on the edition")
	}
	for _, c := range []string{"ageSystem", "ErrMinor"} {
		if !strings.Contains(src, c) {
			t.Errorf("describe.go lost %s", c)
		}
	}
	k := strings.Index(src, "var ageSystem")
	if k < 0 {
		k = strings.Index(src, "const ageSystem")
	}
	if k < 0 {
		t.Fatal("ageSystem is gone: update this test")
	}
	text := src[k:]
	if e := strings.Index(text[1:], "\n\n"); e >= 0 {
		text = text[:e+1]
	}
	if strings.Contains(text, "edition.") {
		t.Error("the age question depends on the edition")
	}
}

// AllowProviderHostsForTest widens the public list: only test files and the
// edition files may mention it.
func TestTestHookIsOnlyUsedByTests(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/platform/edition/edition_") {
			return
		}
		if b, err := os.ReadFile(filepath.Join(root, rel)); err == nil && strings.Contains(string(b), "AllowProviderHostsForTest") {
			t.Errorf("%s calls AllowProviderHostsForTest, which is for tests only", rel)
		}
	})
}

// The option that starts the server showing every project is named only in private files, tests
// and documentation included: the public branch must not say that it exists.
var showAllOption = regexp.MustCompile("--uncensored|(?m)(?:^|[\\s\"'`(\\[])-uncensored\\b|UNCENSORED")

func TestShowAllOptionOnlyInPrivateFiles(t *testing.T) {
	root := repoRoot(t)
	tagged, list := privateSet(t, root)
	private := map[string]bool{}
	for f := range list {
		private[f] = true
	}
	for _, f := range tagged {
		private[f] = true
	}
	walk(t, root, func(rel string) {
		if private[rel] || strings.HasPrefix(rel, "openspec/") || strings.HasPrefix(rel, "web/src/locale/") || rel == "internal/platform/edition/tree_test.go" {
			return
		}
		switch filepath.Ext(rel) {
		case ".go", ".ts", ".html", ".json", ".yaml", ".sh", ".md", ".yml":
		default:
			return
		}
		if b, err := os.ReadFile(filepath.Join(root, rel)); err == nil {
			if m := showAllOption.Find(b); m != nil {
				t.Errorf("%s names %q, which only private files may", rel, m)
			}
		}
	})
}
