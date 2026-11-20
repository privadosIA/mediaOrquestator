package store

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ErrNotFound is what a repository answers for a row that is not there; handlers map it to 404.
var ErrNotFound = errors.New("not found")

// ErrConflict is what a repository answers for a row whose key exists; handlers map it to 409.
var ErrConflict = errors.New("already exists")

// Repo is a thin generic helper over GORM for the table of T, which has one primary key column. An area wraps it in an interface of
// its own with the queries it needs, and the code that used folders receives that interface.
type Repo[T any] struct {
	db  *gorm.DB
	key string
}

// NewRepo makes the repository of the table of T, whose primary key column is key (for example "id" or "name").
func NewRepo[T any](s *Store, key string) *Repo[T] { return &Repo[T]{db: s.DB, key: key} }

// Get reads the row with the key, or ErrNotFound.
func (r *Repo[T]) Get(id any) (*T, error) {
	var v T
	err := r.db.Where(r.key+" = ?", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Create inserts a row, or ErrConflict when a row has its key.
func (r *Repo[T]) Create(v *T) error {
	err := r.db.Create(v).Error
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// Save inserts the row or replaces the one that has its key.
func (r *Repo[T]) Save(v *T) error { return r.db.Save(v).Error }

// Delete deletes the row with the key, or ErrNotFound when there is none.
func (r *Repo[T]) Delete(id any) error {
	var zero T
	res := r.db.Where(r.key+" = ?", id).Delete(&zero)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Count says how many rows the table has.
func (r *Repo[T]) Count() (int64, error) {
	var n int64
	var zero T
	err := r.db.Model(&zero).Count(&n).Error
	return n, err
}

// isUniqueViolation recognises the error of each engine for a key that exists.
func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "unique constraint") || strings.Contains(m, "duplicate entry") || strings.Contains(m, "duplicate key") || strings.Contains(m, "constraint failed: unique")
}

// Tx runs fn in one transaction: the writes of every repository built on the store it receives are applied together, or none when fn
// returns an error (or panics). Nothing slow may run inside: no model call, no render, no network. A Tx inside a Tx joins the outer one.
func (s *Store) Tx(fn func(tx *Store) error) error {
	if s.root != nil {
		return fn(s)
	}
	return s.DB.Transaction(func(g *gorm.DB) error {
		return fn(&Store{DB: g, Driver: s.Driver, Path: s.Path, reg: s.reg, root: s})
	})
}

func (s *Store) String() string {
	if s.Path != "" {
		return fmt.Sprintf("%s:%s", s.Driver, s.Path)
	}
	return s.Driver
}
