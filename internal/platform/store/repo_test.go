package store

import (
	"errors"
	"testing"
)

type repoItem struct {
	ID    string `gorm:"primaryKey;size:64"`
	Title string
}

func newRepoStore(t *testing.T) (*Store, *Repo[repoItem]) {
	t.Helper()
	s := newMem(t)
	if err := s.DB.AutoMigrate(&repoItem{}); err != nil {
		t.Fatal(err)
	}
	return s, NewRepo[repoItem](s, "id")
}

func TestARepoCreatesReadsSavesCountsAndDeletes(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		_, r := newRepoStore(t)
		if err := r.Create(&repoItem{ID: "a", Title: "one"}); err != nil {
			t.Fatal(err)
		}
		got, err := r.Get("a")
		if err != nil || got.Title != "one" {
			t.Fatalf("%+v %v", got, err)
		}
		if err := r.Save(&repoItem{ID: "a", Title: "uno"}); err != nil {
			t.Fatal(err)
		}
		if err := r.Save(&repoItem{ID: "b", Title: "two"}); err != nil {
			t.Fatal(err)
		}
		if g, _ := r.Get("a"); g.Title != "uno" {
			t.Fatalf("saved title = %q", g.Title)
		}
		if n, err := r.Count(); err != nil || n != 2 {
			t.Fatalf("count %d %v", n, err)
		}
		if err := r.Delete("a"); err != nil {
			t.Fatal(err)
		}
		if n, _ := r.Count(); n != 1 {
			t.Fatalf("count after delete = %d", n)
		}
	})
}

func TestARepoAnswersNotFoundAndConflict(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		_, r := newRepoStore(t)
		if _, err := r.Get("nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get: %v", err)
		}
		if err := r.Delete("nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete: %v", err)
		}
		if err := r.Create(&repoItem{ID: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := r.Create(&repoItem{ID: "a"}); !errors.Is(err, ErrConflict) {
			t.Errorf("Create of an existing key: %v", err)
		}
	})
}

func TestATransactionIsAppliedWholeOrNotAtAll(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s, _ := newRepoStore(t)
		boom := errors.New("boom")
		err := s.Tx(func(tx *Store) error {
			if err := NewRepo[repoItem](tx, "id").Create(&repoItem{ID: "x"}); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		if n, _ := NewRepo[repoItem](s, "id").Count(); n != 0 {
			t.Fatalf("a transaction that failed left %d rows", n)
		}
		err = s.Tx(func(tx *Store) error {
			r := NewRepo[repoItem](tx, "id")
			if err := r.Create(&repoItem{ID: "x"}); err != nil {
				return err
			}
			// a Tx inside a Tx joins the outer one
			return tx.Tx(func(in *Store) error { return NewRepo[repoItem](in, "id").Create(&repoItem{ID: "y"}) })
		})
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := NewRepo[repoItem](s, "id").Count(); n != 2 {
			t.Fatalf("committed %d rows", n)
		}
	})
}

func TestANestedTransactionThatFailsUndoesTheOuterOneToo(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s, _ := newRepoStore(t)
		err := s.Tx(func(tx *Store) error {
			if err := NewRepo[repoItem](tx, "id").Create(&repoItem{ID: "x"}); err != nil {
				return err
			}
			return tx.Tx(func(*Store) error { return errors.New("inner") })
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if n, _ := NewRepo[repoItem](s, "id").Count(); n != 0 {
			t.Fatalf("%d rows survived", n)
		}
	})
}
