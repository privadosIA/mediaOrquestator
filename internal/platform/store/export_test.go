package store

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type expPart struct {
	Name     string `gorm:"primaryKey;size:64"`
	Body     Text
	Doc      JSON[map[string]int]
	Level    int
	Manual   bool
	Made     time.Time
	Finished *time.Time
}

type expEvent struct {
	ID   uint   `gorm:"primaryKey"`
	Line string `json:"-"`
}

var expModels = []any{&expPart{}, &expEvent{}, &DataImport{}}

func newExpStore(t *testing.T) *Store { return migratedExp(t, newMem(t)) }

// newExpStoreSecond is a second database of the same engine, for a restore or a copy.
func newExpStoreSecond(t *testing.T) *Store { return migratedExp(t, newSecond(t)) }

func migratedExp(t *testing.T, s *Store) *Store {
	t.Helper()
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.AutoMigrate(&expPart{}, &expEvent{}); err != nil {
		t.Fatal(err)
	}
	return s
}

func seedExp(t *testing.T, s *Store) {
	t.Helper()
	fin := time.Date(2026, 11, 18, 10, 0, 0, 123000000, time.UTC)
	for i := 0; i < 3; i++ {
		p := expPart{Name: fmt.Sprintf("p%d", i), Body: Text(strings.Repeat("é<>&\n\"", 50)), Doc: JSON[map[string]int]{V: map[string]int{"a": i}}, Level: i, Manual: i == 1, Made: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
		if i == 2 {
			p.Finished = &fin
		}
		if err := s.DB.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 1200; i++ {
		if err := s.DB.Create(&expEvent{Line: fmt.Sprintf("line %d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DB.Create(&DataImport{Entity: "x", State: "done", Created: 4, StartedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
}

func export(t *testing.T, s *Store) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := s.exportModels(&buf, expModels); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnExportIsAZipWithAManifestAndAFilePerTable(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newExpStore(t)
		seedExp(t, s)
		zr, err := zip.NewReader(bytes.NewReader(export(t, s)), int64(len(export(t, s))))
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]*zip.File{}
		for _, f := range zr.File {
			names[f.Name] = f
		}
		for _, n := range []string{"manifest.json", "exp_parts.jsonl", "exp_events.jsonl", "data_imports.jsonl"} {
			if names[n] == nil {
				t.Fatalf("missing %s in %v", n, names)
			}
		}
		var man Manifest
		if err := readJSONFile(names["manifest.json"], &man); err != nil {
			t.Fatal(err)
		}
		rows := map[string]int{}
		for _, mt := range man.Tables {
			rows[mt.Name] = mt.Rows
		}
		if rows["exp_parts"] != 3 || rows["exp_events"] != 1200 || rows["data_imports"] != 1 || man.Driver != s.Driver || len(man.Migrations) < 1 || man.ExportedAt == "" {
			t.Fatalf("%+v", man)
		}
		rc, _ := names["exp_events.jsonl"].Open()
		b, _ := io.ReadAll(rc)
		if !strings.Contains(string(b), `"line":"line 0"`) {
			t.Errorf("a column whose json tag is - must be exported by its column name: %s", b[:80])
		}
	})
}

func TestAnExportRestoresIntoAnEmptyDatabaseWithTheSameRows(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		src := newExpStore(t)
		seedExp(t, src)
		data := export(t, src)
		dst := newExpStoreSecond(t)
		if err := dst.restoreModels(bytes.NewReader(data), int64(len(data)), expModels, dst.reg.Migrations()); err != nil {
			t.Fatal(err)
		}
		var parts []expPart
		dst.DB.Order("name").Find(&parts)
		if len(parts) != 3 || parts[2].Finished == nil || !parts[2].Finished.Equal(time.Date(2026, 11, 18, 10, 0, 0, 123000000, time.UTC)) || parts[1].Manual != true || parts[2].Doc.V["a"] != 2 || parts[0].Body != Text(strings.Repeat("é<>&\n\"", 50)) || parts[0].Finished != nil {
			t.Fatalf("%+v", parts)
		}
		var n int64
		dst.DB.Model(&expEvent{}).Count(&n)
		var last expEvent
		dst.DB.Order("id desc").First(&last)
		if n != 1200 || last.Line != "line 1199" || last.ID != 1200 {
			t.Fatalf("events %d last %+v", n, last)
		}
		var imp DataImport
		if err := dst.DB.First(&imp, "entity = ?", "x").Error; err != nil || imp.Created != 4 {
			t.Fatalf("the records of the imports come with the export: %+v %v", imp, err)
		}
		// a second export of the restored database has the same rows
		again := export(t, dst)
		zr, _ := zip.NewReader(bytes.NewReader(again), int64(len(again)))
		zo, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		sizes := func(z *zip.Reader) map[string]int {
			m := map[string]int{}
			for _, f := range z.File {
				if strings.HasSuffix(f.Name, ".jsonl") {
					m[f.Name] = int(f.UncompressedSize64)
				}
			}
			return m
		}
		a, b := sizes(zr), sizes(zo)
		for k := range b {
			if a[k] != b[k] {
				t.Errorf("%s: %d bytes after the round trip, %d before", k, a[k], b[k])
			}
		}
		// the numbering goes on after the restored ids (Postgres needs its sequence moved)
		next := expEvent{Line: "after the restore"}
		if err := dst.DB.Create(&next).Error; err != nil || next.ID != 1201 {
			t.Fatalf("the next event got id %d (%v)", next.ID, err)
		}
	})
}

func TestARestoreIntoADatabaseWithRowsIsRefusedAndChangesNothing(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		src := newExpStore(t)
		seedExp(t, src)
		data := export(t, src)
		dst := newExpStoreSecond(t)
		if err := dst.DB.Create(&expPart{Name: "mine", Made: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		err := dst.restoreModels(bytes.NewReader(data), int64(len(data)), expModels, dst.reg.Migrations())
		if !errors.Is(err, ErrNotEmpty) {
			t.Fatalf("err = %v", err)
		}
		var n int64
		dst.DB.Model(&expPart{}).Count(&n)
		dst.DB.Model(&expEvent{}).Count(new(int64))
		if n != 1 {
			t.Fatalf("%d parts: nothing must change", n)
		}
	})
}

func TestAnExportOfANewerProgramIsRefusedNamingItsMigrations(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		src := newExpStore(t)
		seedExp(t, src)
		src.DB.Exec("INSERT INTO schema_migrations (id) VALUES ('209901010000_from_the_future')")
		data := export(t, src)
		dst := newExpStoreSecond(t)
		err := dst.restoreModels(bytes.NewReader(data), int64(len(data)), expModels, dst.reg.Migrations())
		if !errors.Is(err, ErrNewerExport) || !strings.Contains(err.Error(), "209901010000_from_the_future") {
			t.Fatalf("err = %v", err)
		}
		if err := dst.restoreModels(bytes.NewReader([]byte("not a zip")), 9, expModels, dst.reg.Migrations()); err == nil {
			t.Fatal("something that is not a zip is refused")
		}
	})
}

func TestARestoreThatFailsHalfWayLeavesTheDatabaseEmpty(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		src := newExpStore(t)
		seedExp(t, src)
		data := export(t, src)
		dst := newExpStoreSecond(t)
		// a table of the target that cannot take the rows: the events table does not exist there
		dst.DB.Migrator().DropTable(&expEvent{})
		err := dst.restoreModels(bytes.NewReader(data), int64(len(data)), expModels, dst.reg.Migrations())
		if err == nil {
			t.Fatal("expected an error")
		}
		var n int64
		dst.DB.Model(&expPart{}).Count(&n)
		if n != 0 {
			t.Fatalf("the parts of the failed restore are still there: %d", n)
		}
	})
}

func TestCopyTablesCopiesWhatIsMissingAndReportsEachTableAndASecondCopyCopiesNothing(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		src := newExpStore(t)
		seedExp(t, src)
		dst := newExpStoreSecond(t)
		dst.DB.Create(&expPart{Name: "p1", Level: 99, Made: time.Now()})
		res, err := copyModels(src, dst, expModels, map[string]bool{"data_imports": true})
		if err != nil {
			t.Fatal(err)
		}
		if res["exp_parts"].Copied != 2 || res["exp_parts"].Existing != 1 || res["exp_events"].Copied != 1200 {
			t.Fatalf("%+v", res)
		}
		if _, ok := res["data_imports"]; ok {
			t.Fatal("a skipped table is not copied")
		}
		var p expPart
		dst.DB.First(&p, "name = ?", "p1")
		if p.Level != 99 {
			t.Fatal("a row that is already there is left as it is")
		}
		res, err = copyModels(src, dst, expModels, map[string]bool{"data_imports": true})
		if err != nil || res["exp_parts"].Copied != 0 || res["exp_events"].Copied != 0 || res["exp_events"].Existing != 1200 {
			t.Fatalf("%+v %v", res, err)
		}
	})
}

var _ = gorm.ErrRecordNotFound

func TestTheStatusOfADatabaseNotMigratedYetCountsAMissingTableAsZero(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newMem(t)
		if err := s.DB.AutoMigrate(&expPart{}); err != nil {
			t.Fatal(err)
		}
		counts, err := s.TableCounts()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range counts {
			if c.Name == "data_imports" && c.Rows != 0 {
				t.Fatalf("%+v", c)
			}
		}
		st, err := s.Status()
		if err != nil || len(st.Migrations.Pending) == 0 || len(st.Migrations.Applied) != 0 {
			t.Fatalf("%+v %v", st, err)
		}
	})
}
