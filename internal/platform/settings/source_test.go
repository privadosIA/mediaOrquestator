package settings

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// fakeSource keeps the File it is given in memory, as a database would; it leaves the bootstrap fields out, as a Source must.
type fakeSource struct {
	f       File
	loadErr error
	saved   int
}

func (s *fakeSource) Load() (File, error) {
	if s.loadErr != nil {
		return File{}, s.loadErr
	}
	return s.f, nil
}

func (s *fakeSource) Save(f File) error {
	s.saved++
	f.Database, f.ServerName = Database{}, ""
	s.f = f
	return nil
}

func useFake(t *testing.T) *fakeSource {
	t.Helper()
	fs := &fakeSource{}
	UseSource(fs)
	t.Cleanup(func() { UseSource(nil) })
	return fs
}

func writeSettingsFile(t *testing.T, doc string) {
	t.Helper()
	if err := writePrivate(Path(), []byte(doc)); err != nil {
		t.Fatal(err)
	}
}

func TestSource_LoadTakesTheBootstrapFieldsFromTheFileAndTheRestFromTheSource(t *testing.T) {
	isolate(t)
	writeSettingsFile(t, `{"database": {"driver": "mysql", "host": "db", "password": "p"}, "server_name": "box", "ollama_host": "http://file:11434"}`)
	fs := useFake(t)
	fs.f = File{OllamaHost: "http://source:11434", CORSOrigins: []string{"http://a"}}
	got := Load()
	if got.Database.Driver != "mysql" || got.Database.Password != "p" || got.ServerName != "box" {
		t.Fatalf("the bootstrap fields come from the file: %+v", got)
	}
	if got.OllamaHost != "http://source:11434" || len(got.CORSOrigins) != 1 {
		t.Fatalf("the rest comes from the source, not from the file: %+v", got)
	}
	if h := Ollama(); h.Host != "http://source:11434" || h.Source != SourceSetting {
		t.Fatalf("what is built on Load sees the source: %+v", h)
	}
}

func TestSource_SaveSplitsTheFileFromTheSourceAndKeepsWhatTheFileHeld(t *testing.T) {
	isolate(t)
	writeSettingsFile(t, `{"ollama_host": "http://old:11434", "heartmula_token": "kept-as-is"}`)
	fs := useFake(t)
	f := Load()
	f.OllamaHost = "http://new:11434"
	f.ServerName = "box"
	f.Database = Database{Driver: DBPostgres, Host: "db"}
	if err := Save(f); err != nil {
		t.Fatal(err)
	}
	if fs.f.OllamaHost != "http://new:11434" || fs.f.ServerName != "" || fs.f.Database != (Database{}) {
		t.Fatalf("the source got the settings and not the bootstrap fields: %+v", fs.f)
	}
	raw, _ := os.ReadFile(Path())
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["server_name"] != "box" || doc["database"] == nil {
		t.Fatalf("the file got the bootstrap fields: %s", raw)
	}
	if doc["ollama_host"] != "http://old:11434" || doc["heartmula_token"] != "kept-as-is" {
		t.Fatalf("what the file held and the source now owns is left as it was (a previous version can still read it): %s", raw)
	}
	// a save that changes nothing of the bootstrap does not rewrite the file
	st1, _ := os.Stat(Path())
	if err := Save(Load()); err != nil {
		t.Fatal(err)
	}
	if st2, _ := os.Stat(Path()); !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("the file was rewritten for nothing")
	}
	// clearing both bootstrap fields leaves the other keys; a file with nothing left goes
	f = Load()
	f.ServerName, f.Database = "", Database{}
	if err := Save(f); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(Path())
	doc = map[string]any{}
	if _ = json.Unmarshal(raw, &doc); doc["server_name"] != nil || doc["ollama_host"] == nil {
		t.Fatalf("%s", raw)
	}
	_ = os.WriteFile(Path(), []byte(`{"server_name": "x"}`), 0o600)
	if err := Save(File{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path()); !os.IsNotExist(err) {
		t.Fatalf("an empty file stays: %v", err)
	}
}

func TestSource_AFailureOfTheSourceIsNotNoSettingsForAReaderThatAsksForTheError(t *testing.T) {
	isolate(t)
	fs := useFake(t)
	fs.f = File{CORSOrigins: []string{"http://a"}}
	fs.loadErr = errors.New("database is down")
	if _, err := ReadFile(); err == nil {
		t.Fatal("ReadFile hides the failure of the source")
	}
	if got := Load(); len(got.CORSOrigins) != 0 {
		t.Fatalf("Load answers no settings when the source fails: %+v", got)
	}
	// and without a source it is the file again
	UseSource(nil)
	if err := Save(File{OllamaHost: "http://file:11434"}); err != nil {
		t.Fatal(err)
	}
	if Load().OllamaHost != "http://file:11434" {
		t.Fatal("without a source the settings are the file")
	}
}
