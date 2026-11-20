package store

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Registry is what a database is told about the program: the models of its tables (what the export, the restore, the copy to another engine
// and the status walk), the migrations that create and change them, and the importers of the state that was kept in files. It is built in one
// place, when the program starts, by calling what each area offers (see NewRegistry and the Register functions of the areas), and given to Open.
// Nothing registers itself: a package that is not asked is not there, and a test builds the registry it needs.
type Registry struct {
	mu         sync.Mutex
	models     []any
	seen       map[reflect.Type]bool
	migrations []*Migration
	importers  []Importer
}

// NewRegistry makes a registry with what this package owns: the table of the records of the imports and its migration.
func NewRegistry() *Registry {
	r := &Registry{seen: map[reflect.Type]bool{}}
	r.AddModels(&DataImport{})
	r.AddMigration(dataImportsMigration())
	return r
}

// AddModels adds models (pointers to the structs of the tables), in the order given: a table that others refer to is added before them. A
// model added twice is a panic, because it is a mistake of the program that shows at start.
func (r *Registry) AddModels(models ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range models {
		t := reflect.TypeOf(m)
		if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
			panic(fmt.Sprintf("store: a model must be a pointer to a struct, got %T", m))
		}
		if r.seen[t] {
			panic(fmt.Sprintf("store: the model %s is registered twice", t.Elem().Name()))
		}
		r.seen[t] = true
		r.models = append(r.models, m)
	}
}

// AddMigration adds a migration. The id is YYYYMMDDHHMM_short_name; a bad id, a migration with no Migrate and an id added twice are panics.
//
// A migration never uses the live model structs: it declares inside its function a copy of the struct as it is at its date (or calls
// tx.Migrator()), so that it does the same thing forever whatever the models become.
func (r *Registry) AddMigration(m *Migration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.migrations = addMigration(r.migrations, m)
}

// AddImporter adds an importer. An entity added twice is a panic.
func (r *Registry) AddImporter(i Importer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, o := range r.importers {
		if o.Entity() == i.Entity() {
			panic(fmt.Sprintf("store: the importer of %s is registered twice", i.Entity()))
		}
	}
	r.importers = append(r.importers, i)
}

// Models returns the models in the order they were added.
func (r *Registry) Models() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]any(nil), r.models...)
}

// Migrations returns the migrations sorted by id.
func (r *Registry) Migrations() []*Migration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]*Migration(nil), r.migrations...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Importers returns the importers in the order they were added.
func (r *Registry) Importers() []Importer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Importer(nil), r.importers...)
}
