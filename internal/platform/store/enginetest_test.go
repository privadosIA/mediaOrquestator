package store

// enginetest_test.go: the store tests run on every engine that is available: SQLite in memory always, MySQL and Postgres when a database is
// given in STORE_TEST_MYSQL_DSN or STORE_TEST_POSTGRES_DSN (see docs/DATA_STORE.md). eachEngine runs a test once per engine as the subtests
// sqlite, mysql and postgres; newMem gives the store of the engine its subtest is for. Each test of a remote engine starts by dropping every
// table of the database, so the database must be one of its own.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var engineDSN = map[string]string{"mysql": "STORE_TEST_MYSQL_DSN", "postgres": "STORE_TEST_POSTGRES_DSN"}

// eachEngine runs fn as a subtest for each engine; one that has no database given is skipped, and -v says so.
func eachEngine(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if v, ok := engineDSN[engine]; ok && os.Getenv(v) == "" {
				t.Skipf("%s is not set: the %s tests are skipped", v, engine)
			}
			fn(t)
		})
	}
}

// engineOf is the engine the subtest t is for.
func engineOf(t *testing.T) string {
	name := t.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return "sqlite"
}

// newMem opens the database of the engine the test is running for, empty: SQLite in memory, or the MySQL or Postgres database of the DSN with
// every table dropped (and dropped again when the test ends).
func newMem(t *testing.T) *Store { return newEngineStore(t, false) }

// newSecond opens a second, separate and empty database of the engine the test is for: what a test of a restore or of a copy between two
// databases needs. For MySQL and Postgres it is the database of the DSN with `_b` added to its name, created if it does not exist.
func newSecond(t *testing.T) *Store { return newEngineStore(t, true) }

func newEngineStore(t *testing.T, second bool) *Store {
	t.Helper()
	engine := engineOf(t)
	dsn := os.Getenv(engineDSN[engine])
	if second && dsn != "" {
		dsn = secondDSN(t, engine, dsn)
	}
	if dsn == "" {
		s, err := OpenMemory(nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	db, err := gorm.Open(dialectorOf(engine, dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("cannot open the %s test database: %v", engine, err)
	}
	s := &Store{DB: db, Driver: engine, reg: NewRegistry()}
	dropAll(t, s)
	t.Cleanup(func() {
		dropAll(t, s)
		s.Close()
	})
	return s
}

// dropAll drops every table of the test database.
func dropAll(t *testing.T, s *Store) {
	t.Helper()
	if s.Driver == "mysql" {
		s.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")
		defer s.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
	}
	tables, err := s.DB.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		if err := s.DB.Migrator().DropTable(name); err != nil {
			t.Fatalf("cannot drop %s: %v", name, err)
		}
	}
}

var (
	reMySQLName    = regexp.MustCompile(`/([A-Za-z0-9_]+)(\?|$)`)
	rePostgresName = regexp.MustCompile(`dbname=([A-Za-z0-9_]+)`)
)

// secondDSN makes the DSN of the second database of an engine and creates the database.
func secondDSN(t *testing.T, engine, dsn string) string {
	t.Helper()
	admin, err := gorm.Open(dialectorOf(engine, dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("cannot open the %s test database: %v", engine, err)
	}
	defer func() {
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if engine == "mysql" {
		m := reMySQLName.FindStringSubmatch(dsn)
		name := m[1] + "_b"
		if err := admin.Exec("CREATE DATABASE IF NOT EXISTS " + name).Error; err != nil {
			t.Fatalf("cannot create %s: %v", name, err)
		}
		return strings.Replace(dsn, "/"+m[1], "/"+name, 1)
	}
	m := rePostgresName.FindStringSubmatch(dsn)
	name := m[1] + "_b"
	var n int64
	admin.Raw("SELECT count(*) FROM pg_database WHERE datname = ?", name).Scan(&n)
	if n == 0 {
		if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
			t.Fatalf("cannot create %s: %v", name, err)
		}
	}
	return strings.Replace(dsn, "dbname="+m[1], "dbname="+name, 1)
}

func dialectorOf(engine, dsn string) gorm.Dialector {
	if engine == "mysql" {
		return mysql.Open(dsn)
	}
	return postgres.Open(dsn)
}

// sameDatabase opens a second handle on the database s is (MySQL or Postgres only), without dropping anything: two programs on one database.
func sameDatabase(t *testing.T, s *Store) *Store {
	t.Helper()
	dsn := os.Getenv(engineDSN[s.Driver])
	db, err := gorm.Open(dialectorOf(s.Driver, dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return &Store{DB: db, Driver: s.Driver, reg: s.reg}
}
