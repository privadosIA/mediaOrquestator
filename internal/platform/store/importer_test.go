package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type impRow struct {
	ID   string `gorm:"primaryKey;size:64"`
	Text string
}

// fakeImporter plays an entity whose "files" are the entries of files: a value that starts with "bad" cannot be read.
type fakeImporter struct {
	name    string
	after   []string
	files   map[string]string
	imports int
	fail    error
	hold    chan struct{} // when set, Import waits for it
	started chan struct{}
}

func (f *fakeImporter) Entity() string  { return f.name }
func (f *fakeImporter) After() []string { return f.after }
func (f *fakeImporter) LegacyFiles() ([]LegacyFile, error) {
	var out []LegacyFile
	for p, v := range f.files {
		out = append(out, LegacyFile{Path: p, Bytes: int64(len(v))})
	}
	return out, nil
}
func (f *fakeImporter) Import(_ context.Context, tx *Store) (Report, error) {
	f.imports++
	if f.started != nil {
		close(f.started)
	}
	if f.hold != nil {
		<-f.hold
	}
	var rows []impRow
	var rep Report
	for p, v := range f.files {
		if strings.HasPrefix(v, "bad") {
			rep.Skipped = append(rep.Skipped, Skipped{Path: p, Reason: "not valid"})
			continue
		}
		rows = append(rows, impRow{ID: p, Text: v})
	}
	var err error
	rep.Created, rep.Existing, err = InsertMissing(tx, rows)
	if err != nil {
		return rep, err
	}
	return rep, f.fail
}

func newImportStore(t *testing.T) *Store {
	t.Helper()
	s := newMem(t)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.AutoMigrate(&impRow{}); err != nil {
		t.Fatal(err)
	}
	return s
}

func files(n int) map[string]string {
	m := map[string]string{}
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("f%03d", i)] = "ok"
	}
	return m
}

func count(t *testing.T, s *Store) int64 {
	t.Helper()
	var n int64
	s.DB.Model(&impRow{}).Count(&n)
	return n
}

func TestTheFirstStartImportsTheFilesAndTheSecondImportsNothing(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		imp := &fakeImporter{name: "things", files: files(100)}
		recs, err := s.runImports(context.Background(), "", []Importer{imp})
		if err != nil || len(recs) != 1 || recs[0].State != ImportDone || recs[0].Created != 100 {
			t.Fatalf("%+v %v", recs, err)
		}
		if count(t, s) != 100 {
			t.Fatalf("%d rows", count(t, s))
		}
		// the user changes a row meanwhile: the second start must not touch it
		s.DB.Model(&impRow{}).Where("id = ?", "f001").Update("text", "edited")
		recs, err = s.runImports(context.Background(), "", []Importer{imp})
		if err != nil || len(recs) != 0 || imp.imports != 1 {
			t.Fatalf("second start: %+v %v imports=%d", recs, err, imp.imports)
		}
		var r impRow
		s.DB.First(&r, "id = ?", "f001")
		if r.Text != "edited" {
			t.Fatal("a row changed after the import must stay as it was left")
		}
	})
}

func TestAFileThatCannotBeReadIsSkippedAndNamedAndTheOthersAreImported(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		f := files(3)
		f["broken"] = "bad file"
		imp := &fakeImporter{name: "things", files: f}
		recs, _ := s.runImports(context.Background(), "", []Importer{imp})
		if recs[0].State != ImportDone || recs[0].Created != 3 || len(recs[0].Skipped) != 1 || recs[0].Skipped[0].Path != "broken" || recs[0].Skipped[0].Reason == "" {
			t.Fatalf("%+v", recs[0])
		}
		stored, _ := s.ImportRecords()
		if len(stored) != 1 || len(stored[0].Skipped) != 1 || stored[0].FinishedAt == nil {
			t.Fatalf("the record is in the database: %+v", stored)
		}
	})
}

func TestAskingForAnImportRunsItAgainAndCreatesOnlyWhatIsMissing(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		imp := &fakeImporter{name: "things", files: files(3)}
		if _, err := s.runImports(context.Background(), "", []Importer{imp}); err != nil {
			t.Fatal(err)
		}
		imp.files["new"] = "ok"
		recs, err := s.runImports(context.Background(), "things", []Importer{imp})
		if err != nil || recs[0].Created != 1 || recs[0].Existing != 3 || count(t, s) != 4 {
			t.Fatalf("%+v %v rows %d", recs, err, count(t, s))
		}
		if _, err := s.runImports(context.Background(), "nothing", []Importer{imp}); !errors.Is(err, ErrUnknownEntity) || !strings.Contains(err.Error(), "things") {
			t.Fatalf("an unknown entity names the ones that exist: %v", err)
		}
	})
}

func TestAFailingImportIsUndoneRecordedAndDoesNotStopTheOthers(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		bad := &fakeImporter{name: "a-bad", files: map[string]string{"x1": "ok"}, fail: errors.New("boom")}
		good := &fakeImporter{name: "b-good", files: map[string]string{"y1": "ok"}}
		recs, err := s.runImports(context.Background(), "", []Importer{bad, good})
		if err != nil || len(recs) != 2 {
			t.Fatalf("%+v %v", recs, err)
		}
		if recs[0].State != ImportFailed || !strings.Contains(recs[0].Error, "boom") || recs[1].State != ImportDone {
			t.Fatalf("%+v", recs)
		}
		var n int64
		s.DB.Model(&impRow{}).Where("id = ?", "x1").Count(&n)
		if n != 0 {
			t.Fatal("what a failed import created must be undone")
		}
		// the next start tries the failed one again and not the one that is done
		bad.fail = nil
		recs, _ = s.runImports(context.Background(), "", []Importer{bad, good})
		if len(recs) != 1 || recs[0].Entity != "a-bad" || recs[0].State != ImportDone || good.imports != 1 {
			t.Fatalf("%+v", recs)
		}
	})
}

func TestImportersRunAfterTheOnesTheyNameWhateverTheRegistrationOrder(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		var order []string
		mk := func(name string, after ...string) *orderImporter {
			return &orderImporter{fakeImporter{name: name, after: after}, &order}
		}
		child, parent := mk("library", "projects"), mk("projects")
		if _, err := s.runImports(context.Background(), "", []Importer{child, parent}); err != nil {
			t.Fatal(err)
		}
		if strings.Join(order, ",") != "projects,library" {
			t.Fatalf("order = %v", order)
		}
		if _, err := orderImporters([]Importer{mk("a", "b"), mk("b", "a")}); err == nil {
			t.Fatal("a circle of dependencies is an error")
		}
	})
}

type orderImporter struct {
	fakeImporter
	order *[]string
}

func (o *orderImporter) Import(ctx context.Context, tx *Store) (Report, error) {
	*o.order = append(*o.order, o.name)
	return Report{}, nil
}

func TestAnImporterWhoseDependencyFailedIsNotRunAndIsRecordedAsFailed(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		parent := &fakeImporter{name: "projects", files: files(1), fail: errors.New("boom")}
		child := &fakeImporter{name: "library", after: []string{"projects"}, files: map[string]string{"s1": "ok"}}
		recs, _ := s.runImports(context.Background(), "", []Importer{child, parent})
		if len(recs) != 2 || recs[1].Entity != "library" || recs[1].State != ImportFailed || !strings.Contains(recs[1].Error, "projects") || child.imports != 0 {
			t.Fatalf("%+v imports=%d", recs, child.imports)
		}
	})
}

func TestTwoImportsNeverRunAtOnce(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		hold, started := make(chan struct{}), make(chan struct{})
		slow := &fakeImporter{name: "slow", files: files(1), hold: hold, started: started}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.runImports(context.Background(), "", []Importer{slow}); err != nil {
				t.Error(err)
			}
		}()
		<-started
		if _, err := s.runImports(context.Background(), "slow", []Importer{slow}); !errors.Is(err, ErrImportRunning) {
			t.Fatalf("err = %v", err)
		}
		close(hold)
		wg.Wait()
	})
}

func TestInsertMissingCountsWhatItCreatedAndWhatWasThere(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s := newImportStore(t)
		rows := make([]impRow, 1200)
		for i := range rows {
			rows[i] = impRow{ID: fmt.Sprintf("r%04d", i), Text: "a"}
		}
		c, e, err := InsertMissing(s, rows[:600])
		if err != nil || c != 600 || e != 0 {
			t.Fatalf("%d %d %v", c, e, err)
		}
		c, e, err = InsertMissing(s, rows)
		if err != nil || c != 600 || e != 600 || count(t, s) != 1200 {
			t.Fatalf("%d %d %v rows %d", c, e, err, count(t, s))
		}
	})
}
