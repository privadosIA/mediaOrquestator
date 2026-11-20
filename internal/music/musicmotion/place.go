package musicmotion

// place.go: where the documents of a piece are kept (its design, its script, the map of its song and the records of its proposal, its scenes
// and its review). The pipeline and the tests keep them as files in a folder (DirPlace); the server keeps them in the database. A function
// that reads or writes a document of a piece takes the Place, so it does not know which.

import (
	"os"
	"path/filepath"
	"sync"
)

// The kinds of document of a piece.
const (
	DocDesign   = "design"
	DocScript   = "script"
	DocSongMap  = "song_map"
	DocProposal = "proposal"
	DocScenes   = "scenes"
	DocReview   = "review"
)

// DocKinds are all the kinds, in the order the importer reads them.
var DocKinds = []string{DocDesign, DocScript, DocSongMap, DocProposal, DocScenes, DocReview}

// DocFile is the name of the file that keeps a kind of document in a folder.
func DocFile(kind string) string {
	switch kind {
	case DocSongMap:
		return "songmap.json"
	}
	return kind + ".json"
}

// Place is where the documents of one piece are kept. A document is the bytes of its JSON.
type Place interface {
	// ReadDoc reads a document; false when there is none.
	ReadDoc(kind string) ([]byte, bool)
	// WriteDoc keeps a document, replacing the one there was.
	WriteDoc(kind string, body []byte) error
}

// DirPlace keeps the documents of a piece as files in its folder.
type DirPlace string

// ReadDoc reads the file of a kind.
func (d DirPlace) ReadDoc(kind string) ([]byte, bool) {
	raw, err := os.ReadFile(filepath.Join(string(d), DocFile(kind)))
	return raw, err == nil
}

// WriteDoc writes the file of a kind, creating the folder.
func (d DirPlace) WriteDoc(kind string, body []byte) error {
	if err := os.MkdirAll(string(d), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(string(d), DocFile(kind)), body, 0o644)
}

// MemPlace keeps the documents in memory: a piece that is being made collects them here and keeps them all together, or none, when it is done.
type MemPlace struct {
	mu   sync.Mutex
	Docs map[string][]byte
}

// NewMemPlace is an empty place in memory.
func NewMemPlace() *MemPlace { return &MemPlace{Docs: map[string][]byte{}} }

// ReadDoc reads a document.
func (m *MemPlace) ReadDoc(kind string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.Docs[kind]
	return b, ok
}

// WriteDoc keeps a document.
func (m *MemPlace) WriteDoc(kind string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Docs[kind] = append([]byte(nil), body...)
	return nil
}
