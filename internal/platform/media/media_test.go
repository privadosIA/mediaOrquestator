package media

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T) (projects, mediaDir string) {
	t.Helper()
	projects, mediaDir = t.TempDir(), t.TempDir()
	p := filepath.Join(projects, "demo")
	write(t, filepath.Join(p, "scenes.yaml"), "project: demo\n")
	write(t, filepath.Join(p, "scenes.yaml.bak"), "old\n")
	write(t, filepath.Join(p, "sources", "a.mp3"), "audio")
	write(t, filepath.Join(p, "out", "demo", "manifest.json"), "{}")
	write(t, filepath.Join(p, "out", "clips", "c.mp4"), "clip")
	write(t, filepath.Join(p, "tmp", "x.wav"), "wav")
	if err := os.Symlink("../../models", filepath.Join(p, "models")); err != nil {
		t.Fatal(err)
	}
	return
}

func TestMigrateMovesEverythingAndKeepsPaths(t *testing.T) {
	projects, mediaDir := project(t)
	rep, err := Migrate(projects, mediaDir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Projects) != 1 || rep.Needed == 0 {
		t.Fatalf("report: %+v", rep)
	}
	p := filepath.Join(projects, "demo")
	f := Folder(mediaDir, "demo")
	for rel, want := range map[string]string{
		"scenes.yaml":            "project: demo\n",
		"out/demo/manifest.json": "{}",
		"out/clips/c.mp4":        "clip",
		"tmp/x.wav":              "wav",
		"sources/a.mp3":          "audio",
	} {
		b, err := os.ReadFile(filepath.Join(p, rel))
		if err != nil || string(b) != want {
			t.Fatalf("%s through the project: %q %v", rel, b, err)
		}
	}
	for _, rel := range []string{"config/scenes.yaml", "config/scenes.yaml.bak", "config/sources/a.mp3", "out/demo/manifest.json", "tmp/x.wav"} {
		if _, err := os.Stat(filepath.Join(f, rel)); err != nil {
			t.Fatalf("%s not in the media folder: %v", rel, err)
		}
	}
	if fi, _ := os.Lstat(filepath.Join(p, "out")); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("out is not a link")
	}
	if _, err := os.Lstat(filepath.Join(p, "models")); err != nil {
		t.Fatal("the models link must stay")
	}
	if _, err := os.Lstat(filepath.Join(p, "scenes.yaml.bak")); err == nil {
		t.Fatal("the backup stays in the project")
	}
	if !Linked(p, mediaDir, "demo") {
		t.Fatal("not linked")
	}
}

func TestMigrateRepeatDoesNothing(t *testing.T) {
	projects, mediaDir := project(t)
	if _, err := Migrate(projects, mediaDir, false, nil); err != nil {
		t.Fatal(err)
	}
	rep, err := Migrate(projects, mediaDir, false, nil)
	if err != nil || len(rep.Projects) != 0 {
		t.Fatalf("second run: %+v %v", rep, err)
	}
}

func TestMigrateDryRunChangesNothing(t *testing.T) {
	projects, mediaDir := project(t)
	rep, err := Migrate(projects, mediaDir, true, nil)
	if err != nil || len(rep.Projects) != 1 || len(rep.Projects[0].Moved) == 0 {
		t.Fatalf("dry run: %+v %v", rep, err)
	}
	if fi, _ := os.Lstat(filepath.Join(projects, "demo", "out")); fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dry run changed the project")
	}
	if _, err := os.Stat(filepath.Join(mediaDir, "demo", "out", "demo")); err == nil {
		t.Fatal("dry run copied files")
	}
}

func TestMigrateSkipsBusyProject(t *testing.T) {
	projects, mediaDir := project(t)
	rep, err := Migrate(projects, mediaDir, false, func(id string) bool { return id == "demo" })
	if err != nil || len(rep.Skipped) != 1 || len(rep.Projects) != 0 {
		t.Fatalf("busy: %+v %v", rep, err)
	}
}

func TestAdoptNewProjectCreatesLinks(t *testing.T) {
	projects, mediaDir := t.TempDir(), t.TempDir()
	p := filepath.Join(projects, "new")
	write(t, filepath.Join(p, "scenes.yaml"), "x")
	if _, err := Adopt(p, mediaDir, "new", false); err != nil {
		t.Fatal(err)
	}
	for _, part := range Parts {
		if !IsLinked(p, mediaDir, "new", part) {
			t.Fatalf("%s not linked", part.Name)
		}
	}
	// A later MkdirAll inside a link lands in the media folder.
	if err := os.MkdirAll(filepath.Join(p, "out", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mediaDir, "new", "out", "new")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveTreeKeepsTheLink(t *testing.T) {
	projects, mediaDir := t.TempDir(), t.TempDir()
	p := filepath.Join(projects, "d")
	write(t, filepath.Join(p, "scenes.yaml"), "x")
	write(t, filepath.Join(p, "tmp", "a", "b.wav"), "w")
	if _, err := Adopt(p, mediaDir, "d", false); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTree(filepath.Join(p, "tmp")); err != nil {
		t.Fatal(err)
	}
	if !IsLinked(p, mediaDir, "d", Parts[5]) {
		t.Fatal("tmp link was removed")
	}
	if es, _ := os.ReadDir(filepath.Join(p, "tmp")); len(es) != 0 {
		t.Fatalf("tmp not empty: %v", es)
	}
}

func TestRemoveProjectAndInvalidID(t *testing.T) {
	m := t.TempDir()
	write(t, filepath.Join(m, "a", "out", "f"), "x")
	write(t, filepath.Join(m, "b", "out", "f"), "x")
	if err := RemoveProject(m, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m, "a")); err == nil {
		t.Fatal("a still there")
	}
	if _, err := os.Stat(filepath.Join(m, "b", "out", "f")); err != nil {
		t.Fatal("b was touched")
	}
	for _, bad := range []string{"", "..", ".", "../b", "a/b"} {
		if RemoveProject(m, bad) == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestCheckRejectsUnwritable(t *testing.T) {
	if _, err := Check(""); err == nil {
		t.Fatal("empty accepted")
	}
	f := filepath.Join(t.TempDir(), "file")
	write(t, f, "x")
	if _, err := Check(filepath.Join(f, "sub")); err == nil {
		t.Fatal("a path under a file accepted")
	}
}
