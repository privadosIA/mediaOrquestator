package store

// importer.go: the import of the state an entity kept in files. Each area that moves an entity to the database registers an Importer
// that says how its files become rows. RunImports runs them when the server starts: only the ones that have not been imported, each
// in one transaction, never overwriting a row and never touching a file, and records what each did in data_imports.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

// The states of an import.
const (
	ImportDone   = "done"
	ImportFailed = "failed"
)

// ErrImportRunning is what an import asked for while another runs answers (a 409 in the API).
var ErrImportRunning = errors.New("another import is running")

// ErrUnknownEntity is what an import of an entity nobody registered answers (a 404 in the API).
var ErrUnknownEntity = errors.New("there is no such entity to import")

// LegacyFile is a state file an entity left on disk after its import.
type LegacyFile struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Skipped is a file an import could not read, and why.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Report is what an Import did.
type Report struct {
	Created  int       `json:"created"`
	Existing int       `json:"existing"`
	Skipped  []Skipped `json:"skipped,omitempty"`
}

// Importer turns the state an entity kept in files into rows.
type Importer interface {
	// Entity names the entity: "music-motion/parts".
	Entity() string
	// Import creates the rows that are missing from the files that exist, through tx (one transaction). It never overwrites a row (see
	// InsertMissing), never changes or deletes a file, and reports a file it cannot read as skipped instead of failing.
	Import(ctx context.Context, tx *Store) (Report, error)
	// LegacyFiles lists the state files of the entity that are on disk: never media, never a bootstrap file, never a secret.
	LegacyFiles() ([]LegacyFile, error)
}

// After is implemented by an Importer that needs the import of other entities to have run first.
type After interface{ After() []string }

// LegacyCleaner is implemented by an Importer whose files are not deleted when they are cleaned (the settings rewrite their file).
// It cleans and says how many bytes it freed.
type LegacyCleaner interface{ CleanLegacy() (int64, error) }

// ImportRecord is the record of the import of one entity.
type ImportRecord struct {
	Entity     string     `json:"entity"`
	State      string     `json:"state"`
	Created    int        `json:"created"`
	Existing   int        `json:"existing"`
	Skipped    []Skipped  `json:"skipped"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func recordOf(d DataImport) ImportRecord {
	r := ImportRecord{Entity: d.Entity, State: d.State, Created: d.Created, Existing: d.Existing, Error: string(d.Error), StartedAt: d.StartedAt.UTC(), FinishedAt: d.FinishedAt}
	r.Skipped = []Skipped{}
	if d.Skipped != "" {
		_ = json.Unmarshal([]byte(d.Skipped), &r.Skipped)
	}
	return r
}

// ImportRecords reads the record of every import that has run, by entity.
func (s *Store) ImportRecords() ([]ImportRecord, error) {
	if !s.DB.Migrator().HasTable(&DataImport{}) {
		return nil, nil // the migration that creates the table has not been applied: nothing has been imported
	}
	var rows []DataImport
	if err := s.DB.Order("entity").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ImportRecord, len(rows))
	for i, r := range rows {
		out[i] = recordOf(r)
	}
	return out, nil
}

func (s *Store) importRecord(entity string) (*DataImport, error) {
	var d DataImport
	err := s.DB.Where("entity = ?", entity).Take(&d).Error
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// RunImports runs the imports. With only == "" it runs, in the order After() asks for, every registered importer that has no record or
// whose record is not done; an importer whose dependency did not end done is not run and is recorded as failed. With an entity it runs
// that one again whatever its state (ErrUnknownEntity when nobody registered it). Each importer runs in one transaction; an error in one
// is recorded and does not stop the others. ErrImportRunning when another import is running. It returns the records of what it ran.
func (s *Store) RunImports(ctx context.Context, only string) ([]ImportRecord, error) {
	return s.runImports(ctx, only, s.reg.Importers())
}

func (s *Store) runImports(ctx context.Context, only string, list []Importer) ([]ImportRecord, error) {
	if !s.importing.TryLock() {
		return nil, ErrImportRunning
	}
	defer s.importing.Unlock()
	byName := map[string]Importer{}
	for _, i := range list {
		byName[i.Entity()] = i
	}
	var todo []Importer
	if only != "" {
		i, ok := byName[only]
		if !ok {
			return nil, fmt.Errorf("%w: %q (the entities are %s)", ErrUnknownEntity, only, strings.Join(entityNames(list), ", "))
		}
		todo = []Importer{i}
	} else {
		ordered, err := orderImporters(list)
		if err != nil {
			return nil, err
		}
		for _, i := range ordered {
			rec, err := s.importRecord(i.Entity())
			if err != nil {
				return nil, err
			}
			if rec == nil || rec.State != ImportDone {
				todo = append(todo, i)
			}
		}
	}
	var out []ImportRecord
	failed := map[string]bool{}
	for _, i := range todo {
		rec := s.runOne(ctx, i, only == "", failed, byName)
		if rec.State != ImportDone {
			failed[i.Entity()] = true
		}
		out = append(out, rec)
		slog.Info("data import", "entity", rec.Entity, "state", rec.State, "created", rec.Created, "existing", rec.Existing, "skipped", len(rec.Skipped), "error", rec.Error)
		if ctx.Err() != nil {
			break
		}
	}
	return out, nil
}

func entityNames(list []Importer) []string {
	names := make([]string, len(list))
	for k, i := range list {
		names[k] = i.Entity()
	}
	sort.Strings(names)
	return names
}

// orderImporters sorts the importers so that each comes after the ones it names in After(); registration order otherwise.
func orderImporters(list []Importer) ([]Importer, error) {
	byName := map[string]Importer{}
	for _, i := range list {
		byName[i.Entity()] = i
	}
	var out []Importer
	state := map[string]int{} // 1: visiting, 2: done
	var visit func(i Importer) error
	visit = func(i Importer) error {
		switch state[i.Entity()] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("the imports of %s depend on each other in a circle", i.Entity())
		}
		state[i.Entity()] = 1
		if a, ok := i.(After); ok {
			for _, dep := range a.After() {
				if d, ok := byName[dep]; ok {
					if err := visit(d); err != nil {
						return err
					}
				}
			}
		}
		state[i.Entity()] = 2
		out = append(out, i)
		return nil
	}
	for _, i := range list {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) runOne(ctx context.Context, i Importer, checkDeps bool, failed map[string]bool, byName map[string]Importer) ImportRecord {
	rec := ImportRecord{Entity: i.Entity(), StartedAt: time.Now().UTC(), Skipped: []Skipped{}}
	finish := func(state, errText string) ImportRecord {
		now := time.Now().UTC()
		rec.State, rec.Error, rec.FinishedAt = state, errText, &now
		skipped, _ := json.Marshal(rec.Skipped)
		row := DataImport{Entity: rec.Entity, State: state, Created: rec.Created, Existing: rec.Existing, Skipped: Text(skipped), Error: Text(errText), StartedAt: rec.StartedAt, FinishedAt: &now}
		if err := s.DB.Save(&row).Error; err != nil {
			slog.Error("data import: cannot record the result", "entity", rec.Entity, "err", err)
		}
		return rec
	}
	if checkDeps {
		if a, ok := i.(After); ok {
			for _, dep := range a.After() {
				if _, known := byName[dep]; !known {
					return finish(ImportFailed, fmt.Sprintf("it needs the import of %s, which does not exist", dep))
				}
				if failed[dep] {
					return finish(ImportFailed, fmt.Sprintf("it needs the import of %s, which did not end well", dep))
				}
				if d, err := s.importRecord(dep); err == nil && (d == nil || d.State != ImportDone) {
					return finish(ImportFailed, fmt.Sprintf("it needs the import of %s, which has not been done", dep))
				}
			}
		}
	}
	var report Report
	err := s.Tx(func(tx *Store) error {
		r, err := i.Import(ctx, tx)
		report = r
		return err
	})
	if err != nil {
		// The transaction was undone: nothing of this import is in the database.
		return finish(ImportFailed, err.Error())
	}
	rec.Created, rec.Existing = report.Created, report.Existing
	if report.Skipped != nil {
		rec.Skipped = report.Skipped
	}
	return finish(ImportDone, "")
}

// InsertMissing inserts the rows whose key does not exist yet, in batches of 500, and says how many it created and how many were already
// there. It never overwrites a row: this is what an Importer uses to be repeatable.
func InsertMissing[T any](tx *Store, rows []T) (created, existing int, err error) {
	const batch = 500
	for start := 0; start < len(rows); start += batch {
		end := start + batch
		if end > len(rows) {
			end = len(rows)
		}
		res := tx.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(rows[start:end])
		if res.Error != nil {
			return created, existing, res.Error
		}
		created += int(res.RowsAffected)
		existing += (end - start) - int(res.RowsAffected)
	}
	return created, existing, nil
}
