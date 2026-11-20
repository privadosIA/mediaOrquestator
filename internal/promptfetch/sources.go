// Package promptfetch fills the store of prompt presets from public datasets: it reads each dataset's licence from its
// card, downloads only the files a source names, reads only the columns it names, cleans and maps the rows, samples with a
// seed and writes presets (or a bundle) with a report of what it did. Nothing here talks to a person or to the server.
package promptfetch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

//go:embed sources.json
var embeddedSources []byte

// TagColumn is a column whose value becomes tags: split on Split when given, in words when Camel.
type TagColumn struct {
	Column string `json:"column"`
	Camel  bool   `json:"camel"`
	Split  string `json:"split"`
}

// FlagColumn is a column that adds Tag when it is true.
type FlagColumn struct {
	Column string `json:"column"`
	Tag    string `json:"tag"`
}

// Columns names every column a source reads; no other column is ever loaded.
type Columns struct {
	Prompt   string       `json:"prompt"`
	SourceID string       `json:"source_id"`
	Tags     []TagColumn  `json:"tags"`
	Flags    []FlagColumn `json:"flags"`
}

// NSFWScore is a column holding a score and the highest value that is kept.
type NSFWScore struct {
	Column string  `json:"column"`
	Max    float64 `json:"max"`
}

// Source is one dataset the command may read.
type Source struct {
	ID              string      `json:"id"`
	Enabled         *bool       `json:"enabled"`
	Repo            string      `json:"repo"`
	Files           []string    `json:"files"`
	Format          string      `json:"format"`
	Kind            string      `json:"kind"`
	Language        string      `json:"language"`
	Engine          string      `json:"engine"`
	Columns         Columns     `json:"columns"`
	ExpectedLicence string      `json:"expected_licence"`
	MaxRows         int         `json:"max_rows"`
	NSFW            []NSFWScore `json:"nsfw"`
	Check           string      `json:"check"`
	Notes           string      `json:"notes"`
}

// On says whether the source is enabled (it is unless the file says false).
func (s Source) On() bool { return s.Enabled == nil || *s.Enabled }

// ColumnsRead is every column the source names: the prompt, the id, those of its tags and flags and its scores.
func (s Source) ColumnsRead() []string {
	var out []string
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	add(s.Columns.Prompt)
	add(s.Columns.SourceID)
	for _, t := range s.Columns.Tags {
		add(t.Column)
	}
	for _, f := range s.Columns.Flags {
		add(f.Column)
	}
	for _, n := range s.NSFW {
		add(n.Column)
	}
	return out
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Validate says what is wrong with a source that is going to run, or nil: the repo and the files become paths and URLs.
func (s Source) Validate() error {
	switch {
	case s.ID == "":
		return fmt.Errorf("a source has no id")
	case !repoRe.MatchString(s.Repo):
		return fmt.Errorf("source %s: %q is not a repo of the form owner/name", s.ID, s.Repo)
	case len(s.Files) == 0:
		return fmt.Errorf("source %s names no file", s.ID)
	case s.Columns.Prompt == "":
		return fmt.Errorf("source %s names no prompt column", s.ID)
	case s.Format != "parquet" && s.Format != "csv" && s.Format != "json":
		return fmt.Errorf("source %s: format must be parquet, csv or json", s.ID)
	case s.ExpectedLicence == "":
		return fmt.Errorf("source %s has no expected_licence", s.ID)
	case s.MaxRows <= 0:
		return fmt.Errorf("source %s: max_rows must be positive", s.ID)
	}
	for _, f := range s.Files {
		if !filepath.IsLocal(f) {
			return fmt.Errorf("source %s: file %q is not a relative path inside the repo", s.ID, f)
		}
	}
	return nil
}

type sourcesFile struct {
	Version int      `json:"version"`
	Sources []Source `json:"sources"`
}

// ParseSources reads a sources file.
func ParseSources(b []byte) ([]Source, error) {
	var f sourcesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("sources file: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("sources file: version %d is not known (1)", f.Version)
	}
	ids := map[string]bool{}
	for _, s := range f.Sources {
		if ids[s.ID] {
			return nil, fmt.Errorf("sources file: the id %q is repeated", s.ID)
		}
		ids[s.ID] = true
	}
	return f.Sources, nil
}

// DefaultSources are the sources built into the program.
func DefaultSources() []Source {
	s, err := ParseSources(embeddedSources)
	if err != nil {
		panic(err) // a test keeps this from happening
	}
	return s
}

// LoadSources reads a sources file from disk.
func LoadSources(path string) ([]Source, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSources(b)
}
