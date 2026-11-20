package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// diskImporter lists the .json files of a folder as its state; the .mp4 beside them is media.
type diskImporter struct {
	fakeImporter
	dir string
}

func (d *diskImporter) LegacyFiles() ([]LegacyFile, error) {
	m, _ := filepath.Glob(filepath.Join(d.dir, "*.json"))
	var out []LegacyFile
	for _, p := range m {
		fi, _ := os.Stat(p)
		out = append(out, LegacyFile{Path: p, Bytes: fi.Size()})
	}
	return out, nil
}

func newDisk(t *testing.T) (*diskImporter, string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.json", "b.json", "clip.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &diskImporter{fakeImporter: fakeImporter{name: "things", files: files(2)}, dir: dir}, dir
}

func TestTheFilesOfAnEntityImportedWithNoErrorsAreCleanedAndItsMediaStays(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		d, dir := newDisk(t)
		list := []Importer{d}
		if _, err := s.runImports(context.Background(), "", list); err != nil {
			t.Fatal(err)
		}
		st, _ := legacyStatuses(list)
		if len(st) != 1 || st[0].Files != 2 || st[0].Bytes != 10 {
			t.Fatalf("%+v", st)
		}
		res, err := s.cleanLegacy([]string{"things"}, list)
		if err != nil || len(res) != 1 || res[0].Deleted != 2 || res[0].Bytes != 10 {
			t.Fatalf("%+v %v", res, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "a.json")); !os.IsNotExist(err) {
			t.Fatal("the state file must be gone")
		}
		if _, err := os.Stat(filepath.Join(dir, "clip.mp4")); err != nil {
			t.Fatal("the media beside the state files must stay")
		}
		if count(t, s) != 2 {
			t.Fatal("the rows must stay")
		}
	})
}

func TestAnEntityWithASkippedFileOrNoImportIsRefusedAndNothingIsDeleted(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		d, dir := newDisk(t)
		d.files["broken"] = "bad"
		list := []Importer{d}
		if _, err := s.cleanLegacy([]string{"things"}, list); !errors.Is(err, ErrLegacyRefused) {
			t.Fatalf("not imported yet: %v", err)
		}
		if _, err := s.runImports(context.Background(), "", list); err != nil {
			t.Fatal(err)
		}
		if _, err := s.cleanLegacy([]string{"things"}, list); !errors.Is(err, ErrLegacyRefused) {
			t.Fatalf("with a skipped file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "a.json")); err != nil {
			t.Fatal("nothing must be deleted when the clean is refused")
		}
		if _, err := s.cleanLegacy([]string{"nope"}, list); !errors.Is(err, ErrUnknownEntity) {
			t.Fatalf("%v", err)
		}
	})
}

func TestACleanThatNamesOneRefusedEntityDeletesNothingOfTheOthers(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		d, dir := newDisk(t)
		other := &fakeImporter{name: "other", files: map[string]string{"x": "bad"}}
		list := []Importer{d, other}
		if _, err := s.runImports(context.Background(), "", list); err != nil {
			t.Fatal(err)
		}
		if _, err := s.cleanLegacy([]string{"things", "other"}, list); !errors.Is(err, ErrLegacyRefused) {
			t.Fatalf("%v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "b.json")); err != nil {
			t.Fatal("all or nothing")
		}
	})
}

type cleanerImporter struct {
	fakeImporter
	cleaned bool
}

func (c *cleanerImporter) CleanLegacy() (int64, error) { c.cleaned = true; return 42, nil }

func TestAnImporterWithItsOwnCleanerIsCleanedByIt(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		c := &cleanerImporter{fakeImporter: fakeImporter{name: "settings", files: files(1)}}
		list := []Importer{c}
		if _, err := s.runImports(context.Background(), "", list); err != nil {
			t.Fatal(err)
		}
		res, err := s.cleanLegacy([]string{"settings"}, list)
		if err != nil || !c.cleaned || res[0].Bytes != 42 {
			t.Fatalf("%+v %v", res, err)
		}
	})
}
