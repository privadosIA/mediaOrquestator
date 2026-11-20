package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStore_PutThenGetGivesTheSameBytesAtAnotherPath(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	write(t, filepath.Join(dir, "a.wav"), "RIFFvoice")
	if err := s.Put("h1", ".wav", filepath.Join(dir, "a.wav")); err != nil {
		t.Fatal(err)
	}
	if !s.Has("h1", ".wav") || s.Has("h1", ".png") || s.Has("other", ".wav") {
		t.Error("Has must be true only for what was put")
	}
	dst := filepath.Join(dir, "tmp", "scene_b.wav")
	if !s.Get("h1", ".wav", dst) {
		t.Fatal("Get missed a stored artifact")
	}
	if got, _ := os.ReadFile(dst); string(got) != "RIFFvoice" {
		t.Errorf("content = %q", got)
	}
}

func TestStore_GetReplacesWhatIsAtTheDestination(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	write(t, filepath.Join(dir, "a.png"), "new")
	_ = s.Put("h", ".png", filepath.Join(dir, "a.png"))
	dst := filepath.Join(dir, "scene.png")
	write(t, dst, "stale")
	if !s.Get("h", ".png", dst) {
		t.Fatal("miss")
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Errorf("content = %q", got)
	}
}

func TestStore_ACopyIsNotALink_RewritingAnArtifactInPlaceDoesNotDamageTheStore(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	src := filepath.Join(dir, "scene.png")
	write(t, src, "original")
	_ = s.Put("h", ".png", src)
	// sd rewrites the picture of the scene in place
	if err := os.WriteFile(src, []byte("overwritten by a later run"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "other.png")
	if !s.Get("h", ".png", dst) {
		t.Fatal("miss")
	}
	if got, _ := os.ReadFile(dst); string(got) != "original" {
		t.Errorf("the store followed the rewritten file: %q", got)
	}
	// and the reverse: rewriting what Get gave does not change the store
	_ = os.WriteFile(dst, []byte("changed"), 0o644)
	again := filepath.Join(dir, "third.png")
	_ = s.Get("h", ".png", again)
	if got, _ := os.ReadFile(again); string(got) != "original" {
		t.Errorf("the store was damaged through a copy: %q", got)
	}
}

func TestStore_AMissingEmptyOrDamagedEntryIsAMissAndIsReplaced(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	dst := filepath.Join(dir, "x.wav")
	if s.Get("nope", ".wav", dst) {
		t.Error("a hash that was never stored is a hit")
	}
	write(t, filepath.Join(s.Dir, "empty.wav"), "")
	if s.Get("empty", ".wav", dst) || s.Has("empty", ".wav") {
		t.Error("an empty stored file is a hit")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "empty.wav")); !os.IsNotExist(err) {
		t.Error("an empty entry must be removed so the phase that runs replaces it")
	}
	// a directory with the name of an entry is not a file
	_ = os.MkdirAll(filepath.Join(s.Dir, "dir.wav"), 0o755)
	if s.Get("dir", ".wav", dst) {
		t.Error("a directory is a hit")
	}
	// replaced by a Put
	write(t, filepath.Join(dir, "good.wav"), "good")
	if err := s.Put("empty", ".wav", filepath.Join(dir, "good.wav")); err != nil || !s.Has("empty", ".wav") {
		t.Errorf("put after a damaged entry: %v", err)
	}
}

func TestStore_AnEmptyOrMissingSourceIsNotStored(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	write(t, filepath.Join(dir, "empty.png"), "")
	if err := s.Put("h", ".png", filepath.Join(dir, "empty.png")); err != nil || s.Has("h", ".png") {
		t.Errorf("an empty file was stored (err %v)", err)
	}
	if err := s.Put("h", ".png", filepath.Join(dir, "missing.png")); err != nil || s.Has("h", ".png") {
		t.Errorf("a missing file was stored (err %v)", err)
	}
}

func TestStore_NilAndEmptyHashesAreHarmless(t *testing.T) {
	var s *Store
	if s.Has("h", ".wav") || s.Get("h", ".wav", "x") || s.Put("h", ".wav", "x") != nil {
		t.Error("a nil store must do nothing")
	}
	real := &Store{Dir: t.TempDir()}
	if real.Has("", ".wav") || real.Get("", ".wav", "x") || real.Put("", ".wav", "x") != nil {
		t.Error("an empty hash must do nothing")
	}
}

func TestStore_OverItsLimitTheOldestFilesGoAndTheNewOneStays(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store"), MaxBytes: 25}
	for i, h := range []string{"a", "b", "c"} {
		src := filepath.Join(dir, h+".png")
		write(t, src, "0123456789") // 10 bytes each
		if err := s.Put(h, ".png", src); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(time.Duration(i-10) * time.Minute)
		_ = os.Chtimes(s.path(h, ".png"), old, old) // a is the oldest
	}
	// the third put brought it to 30 bytes before the touch; put once more to prune with the dates set
	src := filepath.Join(dir, "d.png")
	write(t, src, "0123456789")
	_ = s.Put("d", ".png", src)
	if s.Has("a", ".png") || s.Has("b", ".png") {
		t.Error("the oldest files must have been removed")
	}
	if !s.Has("d", ".png") || !s.Has("c", ".png") {
		t.Error("the newest files must stay")
	}
}

func TestStore_UsingAnEntryProtectsItFromTheNextPrune(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store"), MaxBytes: 25}
	for _, h := range []string{"a", "b"} {
		write(t, filepath.Join(dir, h+".png"), "0123456789")
		_ = s.Put(h, ".png", filepath.Join(dir, h+".png"))
		old := time.Now().Add(-time.Hour)
		_ = os.Chtimes(s.path(h, ".png"), old, old)
	}
	_ = s.Get("a", ".png", filepath.Join(dir, "used.png")) // a is the one in use now
	write(t, filepath.Join(dir, "c.png"), "0123456789")
	_ = s.Put("c", ".png", filepath.Join(dir, "c.png"))
	if !s.Has("a", ".png") || s.Has("b", ".png") {
		t.Errorf("a was used and must stay, b was not and must go (a=%v b=%v)", s.Has("a", ".png"), s.Has("b", ".png"))
	}
}

func TestStore_NoLimitKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: filepath.Join(dir, "store")}
	for _, h := range []string{"a", "b", "c"} {
		write(t, filepath.Join(dir, h), "0123456789")
		_ = s.Put(h, ".png", filepath.Join(dir, h))
	}
	if !s.Has("a", ".png") || !s.Has("b", ".png") || !s.Has("c", ".png") {
		t.Error("without a limit nothing is removed")
	}
}

func TestStoreMaxBytes(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int64
		ok    bool
	}{
		{"", DefaultStoreMaxBytes, true},
		{"  ", DefaultStoreMaxBytes, true},
		{"512", 512 << 20, true},
		{"0", 0, false},
		{"-3", DefaultStoreMaxBytes, true},
		{"lots", DefaultStoreMaxBytes, true},
	} {
		if limit, ok := StoreMaxBytes(tc.in); limit != tc.limit || ok != tc.ok {
			t.Errorf("StoreMaxBytes(%q) = %d, %v; want %d, %v", tc.in, limit, ok, tc.limit, tc.ok)
		}
	}
}
