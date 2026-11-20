package store

// export.go: the export of the data (a .zip with one JSON-lines file per table and a manifest), its restore into an empty database of
// any engine, and the copy of the tables between two open databases. Rows are written by the column names of the models, with the
// type of each field, so that nothing depends on the json tags of the structs. No secret is in a table, so none is in an export.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// ErrNotEmpty is what restoring into a database that has rows answers.
var ErrNotEmpty = errors.New("the database already has rows")

// ErrNewerExport is what restoring an export made by a newer program answers.
var ErrNewerExport = errors.New("the export was made by a newer version of the program")

// Manifest describes an export.
type Manifest struct {
	Migrations []string        `json:"migrations"`
	Driver     string          `json:"driver"`
	ExportedAt string          `json:"exported_at"`
	Tables     []ManifestTable `json:"tables"`
}

// ManifestTable is a table of an export.
type ManifestTable struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

type tableInfo struct {
	model  any
	schema *schema.Schema
}

func (t tableInfo) name() string { return t.schema.Table }

func tablesOf(db *gorm.DB, models []any) ([]tableInfo, error) {
	out := make([]tableInfo, 0, len(models))
	for _, m := range models {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(m); err != nil {
			return nil, fmt.Errorf("cannot read the model %T: %w", m, err)
		}
		out = append(out, tableInfo{model: m, schema: stmt.Schema})
	}
	return out, nil
}

// orderBy is the order the rows of a table are written in: its primary key.
func (t tableInfo) orderBy() string {
	var cols []string
	for _, f := range t.schema.PrimaryFields {
		cols = append(cols, f.DBName)
	}
	return strings.Join(cols, ", ")
}

// forEachRow reads every row of the table through its model, in primary key order, and calls fn with it as a map of column to value.
func (t tableInfo) forEachRow(db *gorm.DB, fn func(row map[string]any) error) error {
	q := db.Model(t.model)
	if o := t.orderBy(); o != "" {
		q = q.Order(o)
	}
	rows, err := q.Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	ctx := context.Background()
	for rows.Next() {
		rv := reflect.New(t.schema.ModelType)
		if err := db.ScanRows(rows, rv.Interface()); err != nil {
			return err
		}
		row := make(map[string]any, len(t.schema.Fields))
		for _, f := range t.schema.Fields {
			if f.DBName == "" {
				continue
			}
			v, _ := f.ValueOf(ctx, rv.Elem())
			row[f.DBName] = v
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Export writes the data of every registered table to w as a .zip: manifest.json and one <table>.jsonl each, a JSON object per line.
// The rows are streamed: memory does not grow with the table.
func (s *Store) Export(w io.Writer) error { return s.exportModels(w, s.reg.Models()) }

func (s *Store) exportModels(w io.Writer, models []any) error {
	tables, err := tablesOf(s.DB, models)
	if err != nil {
		return err
	}
	applied, _, err := s.Migrations()
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	man := Manifest{Driver: s.Driver, ExportedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z")}
	for _, a := range applied {
		man.Migrations = append(man.Migrations, a.ID)
	}
	for _, t := range tables {
		fw, err := zw.Create(t.name() + ".jsonl")
		if err != nil {
			return err
		}
		bw := bufio.NewWriter(fw)
		n := 0
		err = t.forEachRow(s.DB, func(row map[string]any) error {
			b, err := json.Marshal(row)
			if err != nil {
				return err
			}
			_, _ = bw.Write(b) // a buffered writer keeps its first error: Flush reports it
			_ = bw.WriteByte('\n')
			n++
			return nil
		})
		if err != nil {
			return fmt.Errorf("cannot export %s: %w", t.name(), err)
		}
		if err := bw.Flush(); err != nil {
			return err
		}
		man.Tables = append(man.Tables, ManifestTable{Name: t.name(), Rows: n})
	}
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(mw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(man); err != nil {
		return err
	}
	return zw.Close()
}

// importsTable is the table of the records of the imports: a restore brings the records of the export (what the old database had
// imported), replacing the ones the new one made, so it is not part of the "empty" check.
const importsTable = "data_imports"

// Restore loads an export into this database, which must have been migrated and have no rows in any table (the records of the imports
// aside). It refuses, changing nothing, a database with rows (ErrNotEmpty), an export of migrations this program does not know
// (ErrNewerExport) and a table the program has no model of. The whole restore is one transaction.
func (s *Store) Restore(r io.ReaderAt, size int64) error {
	return s.restoreModels(r, size, s.reg.Models(), s.reg.Migrations())
}

func (s *Store) restoreModels(r io.ReaderAt, size int64, models []any, migs []*Migration) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("that is not an export: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	mf, ok := files["manifest.json"]
	if !ok {
		return errors.New("that is not an export: it has no manifest.json")
	}
	var man Manifest
	if err := readJSONFile(mf, &man); err != nil {
		return fmt.Errorf("the manifest cannot be read: %w", err)
	}
	known := map[string]bool{}
	for _, m := range migs {
		known[m.ID] = true
	}
	var unknown []string
	for _, id := range man.Migrations {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: it has migrations this program does not know (%s): update the program", ErrNewerExport, strings.Join(unknown, ", "))
	}
	tables, err := tablesOf(s.DB, models)
	if err != nil {
		return err
	}
	byName := map[string]tableInfo{}
	for _, t := range tables {
		byName[t.name()] = t
	}
	for _, mt := range man.Tables {
		if _, ok := byName[mt.Name]; !ok {
			return fmt.Errorf("the export has the table %s and this program has no such table", mt.Name)
		}
	}
	for _, t := range tables {
		if t.name() == importsTable {
			continue
		}
		var n int64
		if err := s.DB.Model(t.model).Limit(1).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: %s has rows; restore into an empty database", ErrNotEmpty, t.name())
		}
	}
	return s.Tx(func(tx *Store) error {
		for _, t := range tables {
			f, ok := files[t.name()+".jsonl"]
			if !ok {
				continue
			}
			if err := restoreTable(tx.DB, t, f); err != nil {
				return fmt.Errorf("cannot restore %s: %w", t.name(), err)
			}
		}
		if tx.Driver == "postgres" {
			for _, t := range tables {
				if err := fixPostgresSequence(tx.DB, t); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func readJSONFile(f *zip.File, v any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(v)
}

// restoreTable inserts the rows of a .jsonl file in batches of 500. Each cell is decoded into the type of its field.
func restoreTable(db *gorm.DB, t tableInfo, f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 1<<20), 1<<30)
	var batch []map[string]any
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		q := db.Model(t.model)
		if t.name() == importsTable {
			q = q.Clauses(clause.OnConflict{UpdateAll: true})
		}
		err := q.Create(&batch).Error
		batch = batch[:0]
		return err
	}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			return err
		}
		row := make(map[string]any, len(raw))
		for col, cell := range raw {
			fld, ok := t.schema.FieldsByDBName[col]
			if !ok {
				return fmt.Errorf("the column %s is not in the table", col)
			}
			ptr := reflect.New(fld.FieldType)
			if err := json.Unmarshal(cell, ptr.Interface()); err != nil {
				return fmt.Errorf("column %s: %w", col, err)
			}
			row[col] = ptr.Elem().Interface()
		}
		batch = append(batch, row)
		if len(batch) == 500 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

// fixPostgresSequence moves the sequence of an auto-numbered key past the ids a restore inserted: Postgres does not do it for explicit ids.
func fixPostgresSequence(db *gorm.DB, t tableInfo) error {
	if len(t.schema.PrimaryFields) != 1 || !t.schema.PrimaryFields[0].AutoIncrement {
		return nil
	}
	col := t.schema.PrimaryFields[0].DBName
	q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', '%s'), COALESCE(MAX(%s), 1), MAX(%s) IS NOT NULL) FROM %s", t.name(), col, col, col, t.name())
	return db.Exec(q).Error
}

// TableCopy is what copying one table did.
type TableCopy struct {
	Copied   int `json:"copied"`
	Existing int `json:"existing"`
}

// CopyTables copies the rows of the registered tables from src to dst, leaving alone the rows whose key is already in dst, so that it
// can be repeated. The tables named in skip are not copied (a caller that copies them with rules of its own). dst must have been migrated.
func CopyTables(src, dst *Store, skip map[string]bool) (map[string]TableCopy, error) {
	return copyModels(src, dst, src.reg.Models(), skip)
}

func copyModels(src, dst *Store, models []any, skip map[string]bool) (map[string]TableCopy, error) {
	tables, err := tablesOf(src.DB, models)
	if err != nil {
		return nil, err
	}
	out := map[string]TableCopy{}
	err = dst.Tx(func(tx *Store) error {
		for _, t := range tables {
			if skip[t.name()] {
				continue
			}
			res := TableCopy{}
			var batch []map[string]any
			flush := func() error {
				if len(batch) == 0 {
					return nil
				}
				r := tx.DB.Model(t.model).Clauses(clause.OnConflict{DoNothing: true}).Create(&batch)
				if r.Error != nil {
					return r.Error
				}
				res.Copied += int(r.RowsAffected)
				res.Existing += len(batch) - int(r.RowsAffected)
				batch = batch[:0]
				return nil
			}
			err := t.forEachRow(src.DB, func(row map[string]any) error {
				batch = append(batch, row)
				if len(batch) == 500 {
					return flush()
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("%s: %w", t.name(), err)
			}
			if err := flush(); err != nil {
				return fmt.Errorf("%s: %w", t.name(), err)
			}
			out[t.name()] = res
		}
		if tx.Driver == "postgres" {
			for _, t := range tables {
				if skip[t.name()] {
					continue
				}
				if err := fixPostgresSequence(tx.DB, t); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("the copy failed and nothing was changed: %w", err)
	}
	names := make([]string, 0, len(out))
	for n := range out {
		names = append(names, n)
	}
	sort.Strings(names)
	return out, nil
}
