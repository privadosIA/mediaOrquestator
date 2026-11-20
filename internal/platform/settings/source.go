package settings

// source.go: where the settings are kept (change move-settings-and-catalogs-to-database). The package-level functions (Load, Save and everything built
// on them) do not change: they use the Source the process chose with UseSource, which is the database for the server and for the commands that open
// it, and the settings file when none was chosen. The Source is an interface here because the store imports this package for the database settings,
// so this package cannot import the store. The database settings and the server name are read before the database is opened, so they always live
// in the settings file (the bootstrap fields), whatever the source.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// Source keeps the settings that are not read before the database is opened. It is given and returns a whole File; the bootstrap fields of the File
// (Database and ServerName) are not its business: it ignores them on Save and leaves them empty on Load.
type Source interface {
	Load() (File, error)
	Save(File) error
}

var (
	sourceMu sync.RWMutex
	source   Source
)

// UseSource makes every Load and Save of this process use s (nil goes back to the settings file). A program calls it once, when it has opened its
// database, before anything reads a setting; a test that calls it restores the file with UseSource(nil).
func UseSource(s Source) {
	sourceMu.Lock()
	defer sourceMu.Unlock()
	source = s
}

// ReleaseSource goes back to the settings file when s is the Source in use: whoever installed a Source gives it back when its database closes,
// without taking away one that someone installed after it.
func ReleaseSource(s Source) {
	sourceMu.Lock()
	defer sourceMu.Unlock()
	if source == s {
		source = nil
	}
}

// FileSource is the settings file as a Source, for a program that uses the database for its settings only when there is one (see UseSource).
func FileSource() Source { return fileSource{} }

// currentSource is the Source in use, nil when the settings are the file.
func currentSource() Source {
	sourceMu.RLock()
	defer sourceMu.RUnlock()
	return source
}

// fileSource is the settings file: every setting in settings.json, as it has always been.
type fileSource struct{}

// Load reads the settings file; a missing file is no settings, without an error.
func (fileSource) Load() (File, error) {
	p := Path()
	if p == "" {
		return File{}, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("%s: %w", p, err)
	}
	return f, nil
}

// Save writes the whole File to the settings file; a file with nothing left in it is removed.
func (fileSource) Save(f File) error {
	p := Path()
	if p == "" {
		return errors.New("settings: cannot find the config directory")
	}
	if f.empty() {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(p, append(data, '\n'))
}

// Bootstrap are the settings that stay in settings.json when a Source is in use: the ones the program needs to open the database and to name itself
// before it can read anything from it.
type Bootstrap struct {
	Database   Database `json:"database,omitempty"`
	ServerName string   `json:"server_name,omitempty"`
}

// loadBootstrap reads the bootstrap fields of the settings file; a missing file has none.
func loadBootstrap() (Bootstrap, error) {
	var b Bootstrap
	p := Path()
	if p == "" {
		return b, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return b, nil
		}
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return Bootstrap{}, fmt.Errorf("%s: %w", p, err)
	}
	return b, nil
}

// saveBootstrap writes the database settings and the server name to the settings file and touches nothing else in it: until the settings are
// cleaned the file still holds the copy the import read, which a previous version of the program can use. A file left with nothing is removed.
func saveBootstrap(db Database, name string) error {
	p := Path()
	if p == "" {
		return errors.New("settings: cannot find the config directory")
	}
	doc := map[string]json.RawMessage{}
	if data, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	before, _ := json.Marshal(doc)
	set := func(key string, empty bool, v any) error {
		if empty {
			delete(doc, key)
			return nil
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		doc[key] = raw
		return nil
	}
	if err := set("database", db == (Database{}), db); err != nil {
		return err
	}
	if err := set("server_name", name == "", name); err != nil {
		return err
	}
	if after, _ := json.Marshal(doc); string(after) == string(before) {
		return nil // nothing of the bootstrap changed: the file is not rewritten
	}
	if len(doc) == 0 {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(p, append(data, '\n'))
}

// ReadSettingsFile reads settings.json whole, whatever Source is in use: it is what the import of the settings reads. A missing file has no settings
// and is no error.
func ReadSettingsFile() (File, error) { return fileSource{}.Load() }

// cleanedFile is what settings.json holds after cleaning: the bootstrap fields and nothing else.
func cleanedFile() ([]byte, error) {
	b, err := loadBootstrap()
	if err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if b.Database != (Database{}) {
		doc["database"] = b.Database
	}
	if b.ServerName != "" {
		doc["server_name"] = b.ServerName
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// LegacyBytes is how many bytes cleaning settings.json would free: what it holds besides the bootstrap fields. Zero when there is no file.
func LegacyBytes() (int64, error) {
	fi, err := os.Stat(Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	clean, err := cleanedFile()
	if err != nil {
		return 0, err
	}
	if freed := fi.Size() - int64(len(clean)); freed > 0 {
		return freed, nil
	}
	return 0, nil
}

// CleanFile rewrites settings.json with only the database settings and the server name, which are read before the database is opened, and says how
// many bytes that freed. It never deletes the file. Call it only when the settings are in the database: what it takes out is no longer anywhere else.
func CleanFile() (int64, error) {
	freed, err := LegacyBytes()
	if err != nil || freed == 0 {
		return 0, err
	}
	clean, err := cleanedFile()
	if err != nil {
		return 0, err
	}
	return freed, writePrivate(Path(), clean)
}
