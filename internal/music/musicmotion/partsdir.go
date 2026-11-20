package musicmotion

// partsdir.go: the user's parts kept in a folder (one folder per part: manifest.json, part.html, origin.json), the way they were kept before
// the database. It is what a Catalog made with UserDir uses, what the tests build their catalogs on, and what the import reads.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type dirParts struct{ dir string }

func (d dirParts) exists(name string) bool {
	if !reName.MatchString(name) {
		return false
	}
	st, err := os.Stat(filepath.Join(d.dir, name))
	return err == nil && st.IsDir()
}

func (d dirParts) Get(name string) (Part, bool) {
	if !d.exists(name) { // never a path built from a name that is not one
		return Part{}, false
	}
	return readPartFolder(os.DirFS(d.dir), name, "user")
}

func (d dirParts) GetMany(names []string) map[string]Part {
	out := map[string]Part{}
	for _, n := range names {
		if p, ok := d.Get(n); ok {
			out[n] = p
		}
	}
	return out
}

func (d dirParts) List(f SceneFilter) []Part {
	st, err := os.Stat(d.dir)
	if err != nil || !st.IsDir() {
		return nil
	}
	var out []Part
	for _, p := range readParts(os.DirFS(d.dir), "user") {
		if f.matches(p) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (d dirParts) Create(manifest []byte, fragment string, o *PartOrigin) error {
	return WriteUserPart(d.dir, manifest, fragment, o)
}

// Replace writes the new files beside the old ones and renames them, so a failure leaves the old part whole.
func (d dirParts) Replace(name string, manifest []byte, fragment string) error {
	if !d.exists(name) {
		return ErrPartNotFound
	}
	if r, err := rowOf(manifest, fragment, nil); err != nil {
		return err
	} else if r.Name != name {
		return fmt.Errorf("%w: %q instead of %q", ErrPartRenamed, r.Name, name)
	}
	dir := filepath.Join(d.dir, name)
	for file, content := range map[string]string{"manifest.json": string(manifest), "part.html": fragment} {
		if err := os.WriteFile(filepath.Join(dir, file+".new"), []byte(content), 0o644); err != nil {
			return err
		}
	}
	for _, file := range []string{"manifest.json", "part.html"} {
		if err := os.Rename(filepath.Join(dir, file+".new"), filepath.Join(dir, file)); err != nil {
			return err
		}
	}
	return nil
}

func (d dirParts) Delete(name string) error {
	if !d.exists(name) {
		return ErrPartNotFound
	}
	return os.RemoveAll(filepath.Join(d.dir, name))
}

func (d dirParts) DeleteScenes(piece string) (int, error) {
	n := 0
	var errs []error
	for _, p := range d.List(SceneFilter{Scenes: OnlyScenes, Piece: piece}) {
		if err := d.Delete(p.Name); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
