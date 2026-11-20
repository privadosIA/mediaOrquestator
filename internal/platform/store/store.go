// Package store is the data layer of the server: the one database connection of the process (SQLite, MySQL or
// Postgres, the engine chosen in Settings), the registry of the tables, the versioned migrations, the repositories, the
// importer of the state that was kept in files, the listing helper and the export. See openspec/changes/add-data-layer.
package store

import (
	"sync"

	"gorm.io/gorm"
)

// Store is an open database. There is one per process: several connections to one SQLite file fight for its write lock
// and a transaction could not span areas.
type Store struct {
	// DB is the GORM handle. Inside Tx it is the handle of the transaction.
	DB *gorm.DB
	// Driver is the engine: settings.DBSQLite, settings.DBMySQL or settings.DBPostgres.
	Driver string
	// Path is the SQLite file ("" for MySQL and Postgres, and for an in-memory database).
	Path string
	// root is the store a transaction was started from, nil for the store itself.
	root *Store
	// reg is what the program told this database about: its models, migrations and importers (see Registry).
	reg *Registry
	// importing makes two imports never run at once (see RunImports); migrating makes the migrations of this handle take turns (the
	// database lock does it between programs).
	importing, migrating sync.Mutex
}

// Registry is what this database was opened with.
func (s *Store) Registry() *Registry { return s.reg }

// Close closes the connection. A store made by Tx is not closed by this: the transaction ends with its function.
func (s *Store) Close() error {
	if s == nil || s.DB == nil || s.root != nil {
		return nil
	}
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
