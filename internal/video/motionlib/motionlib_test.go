package motionlib

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore_AnEmptyLibraryListsNothingAndIsNotAnError(t *testing.T) {
	eachStore(t, func(t *testing.T, newStore func(dir string) *Store) {
		st := newStore(filepath.Join(t.TempDir(), "motions")) // the folder does not even exist yet
		if got := st.List(); got == nil || len(got) != 0 {
			t.Errorf("list = %#v, want an empty list", got)
		}
		if _, err := st.Get("mot-20260101-000000-abcdef"); !errors.Is(err, ErrNotFound) {
			t.Errorf("get: %v", err)
		}
	})
}

func TestStore_CreateGetListAndDelete(t *testing.T) {
	eachStore(t, func(t *testing.T, newStore func(dir string) *Store) {
		st := newStore(t.TempDir())
		a, err := st.Create("  Baile  ", []string{"dance", "Dance", " ", "latin"})
		if err != nil {
			t.Fatal(err)
		}
		if !ValidID(a.ID) || a.Title != "Baile" || a.State != StateProcessing || len(a.Tags) != 2 {
			t.Errorf("a = %+v", a)
		}
		time.Sleep(2 * time.Millisecond)
		b, _ := st.Create("", nil)
		if b.Title != "motion" || b.Tags == nil {
			t.Errorf("b = %+v: a blank title is called motion and tags are never null", b)
		}
		list := st.List()
		if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
			t.Errorf("list = %+v, want the most recent first", list)
		}
		if err := os.WriteFile(filepath.Join(st.ControlDir(a.ID), "0001.png"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.Delete(a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(st.Path(a.ID)); !os.IsNotExist(err) {
			t.Errorf("the files are still there: %v", err)
		}
		if len(st.List()) != 1 {
			t.Errorf("list = %+v", st.List())
		}
		if err := st.Delete(a.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("second delete: %v", err)
		}
		// a deleted motion is not brought back by a job that finishes late
		a.State = StateReady
		if err := st.Save(a); !errors.Is(err, ErrNotFound) {
			t.Errorf("save after delete: %v", err)
		}
	})
}

func TestCheckText(t *testing.T) {
	if _, _, err := CheckText(strings.Repeat("a", MaxTitle+1), nil); err == nil {
		t.Error("a long title")
	}
	if _, _, err := CheckText("t", []string{strings.Repeat("a", MaxTagLen+1)}); err == nil {
		t.Error("a long tag")
	}
	many := make([]string, MaxTags+1)
	for i := range many {
		many[i] = strings.Repeat("x", i+1)
	}
	if _, _, err := CheckText("t", many); err == nil {
		t.Error("too many tags")
	}
	if title, tags, err := CheckText("ñandú", []string{"a"}); err != nil || title != "ñandú" || len(tags) != 1 {
		t.Errorf("%q %v %v", title, tags, err)
	}
}

func TestMarkInterrupted(t *testing.T) {
	eachStore(t, func(t *testing.T, newStore func(dir string) *Store) {
		st := newStore(t.TempDir())
		m, _ := st.Create("x", nil)
		ready, _ := st.Create("y", nil)
		ready.State = StateReady
		_ = st.Save(ready)
		if n := st.MarkInterrupted(); n != 1 {
			t.Errorf("n = %d", n)
		}
		got, _ := st.Get(m.ID)
		if got.State != StateFailed || !strings.Contains(got.Error, "interrupted") {
			t.Errorf("got = %+v", got)
		}
		if again, _ := st.Get(ready.ID); again.State != StateReady {
			t.Errorf("a ready motion was touched: %+v", again)
		}
	})
}

func TestWindowAt(t *testing.T) {
	eachStore(t, func(t *testing.T, newStore func(dir string) *Store) {
		st := newStore(t.TempDir())
		m, _ := st.Create("x", nil)
		if _, err := st.WindowAt(m.ID, 0); !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "being prepared") {
			t.Errorf("processing: %v", err)
		}
		m.State, m.Error = StateFailed, "no person"
		_ = st.Save(m)
		if _, err := st.WindowAt(m.ID, 0); !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "no person") {
			t.Errorf("failed: %v", err)
		}
		if _, err := st.WindowAt("mot-20260101-000000-abcdef", 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown: %v", err)
		}
		// ten seconds: 160 frames
		m.State, m.Error, m.Duration, m.Frames = StateReady, "", 10, 160
		_ = st.Save(m)
		for _, tc := range []struct {
			start   float64
			ok      bool
			first   int
			from    float64
			wantErr string
		}{
			{0, true, 0, 0, ""}, {4, true, 64, 4, ""}, {7.9, true, 126, 7.875, ""},
			{8, false, 0, 0, "does not fit"}, {9.5, false, 0, 0, "does not fit"}, {-1, false, 0, 0, "does not fit"},
		} {
			w, err := st.WindowAt(m.ID, tc.start)
			if tc.ok {
				if err != nil || w.First != tc.first || w.From != tc.from || w.To != tc.from+float64(WindowFrames)/FPS || w.Dir != st.ControlDir(m.ID) {
					t.Errorf("start %g: %+v, %v", tc.start, w, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("start %g: %+v, %v, want %q", tc.start, w, err, tc.wantErr)
			}
		}
	})
}
