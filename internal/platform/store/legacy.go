package store

// legacy.go: the state files left behind by an entity after its import. They are never deleted by the import: only an explicit
// CleanLegacy, and only of an entity whose import is done with no skipped file. It deletes only the files the importer lists (state,
// never media, a bootstrap file or a secret), one by one, never a folder.

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ErrLegacyRefused is what cleaning the files of an entity whose import is not done, or skipped files, answers (a 409 in the API).
var ErrLegacyRefused = errors.New("these files cannot be cleaned yet")

// LegacyStatus is what an entity has left on disk.
type LegacyStatus struct {
	Entity string `json:"entity"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

// LegacyStatuses says, for each registered importer, how many state files and how many bytes it has left on disk.
func (s *Store) LegacyStatuses() ([]LegacyStatus, error) { return legacyStatuses(s.reg.Importers()) }

func legacyStatuses(list []Importer) ([]LegacyStatus, error) {
	out := []LegacyStatus{}
	for _, i := range list {
		files, err := i.LegacyFiles()
		if err != nil {
			return nil, fmt.Errorf("cannot list the files of %s: %w", i.Entity(), err)
		}
		st := LegacyStatus{Entity: i.Entity(), Files: len(files)}
		for _, f := range files {
			st.Bytes += f.Bytes
		}
		out = append(out, st)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Entity < out[b].Entity })
	return out, nil
}

// CleanResult is what cleaning the files of one entity did.
type CleanResult struct {
	Entity  string `json:"entity"`
	Deleted int    `json:"deleted"`
	Bytes   int64  `json:"bytes"`
}

// CleanLegacy deletes the state files left behind by the entities named. It refuses, changing nothing, if any entity is unknown
// (ErrUnknownEntity), has no import done or an import that skipped files (ErrLegacyRefused).
func (s *Store) CleanLegacy(entities []string) ([]CleanResult, error) {
	return s.cleanLegacy(entities, s.reg.Importers())
}

func (s *Store) cleanLegacy(entities []string, list []Importer) ([]CleanResult, error) {
	byName := map[string]Importer{}
	for _, i := range list {
		byName[i.Entity()] = i
	}
	if len(entities) == 0 {
		return nil, errors.New("name the entities whose files are cleaned")
	}
	var chosen []Importer
	seen := map[string]bool{}
	for _, e := range entities {
		i, ok := byName[e]
		if !ok {
			return nil, fmt.Errorf("%w: %q (the entities are %s)", ErrUnknownEntity, e, strings.Join(entityNames(list), ", "))
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		rec, err := s.importRecord(e)
		if err != nil {
			return nil, err
		}
		switch {
		case rec == nil || rec.State != ImportDone:
			return nil, fmt.Errorf("%w: the import of %s has not ended well", ErrLegacyRefused, e)
		case len(recordOf(*rec).Skipped) > 0:
			return nil, fmt.Errorf("%w: the import of %s skipped %d file(s): they are not in the database", ErrLegacyRefused, e, len(recordOf(*rec).Skipped))
		}
		chosen = append(chosen, i)
	}
	var out []CleanResult
	for _, i := range chosen {
		res := CleanResult{Entity: i.Entity()}
		if c, ok := i.(LegacyCleaner); ok {
			n, err := c.CleanLegacy()
			if err != nil {
				return out, fmt.Errorf("cannot clean %s: %w", i.Entity(), err)
			}
			res.Bytes = n
			out = append(out, res)
			continue
		}
		files, err := i.LegacyFiles()
		if err != nil {
			return out, fmt.Errorf("cannot list the files of %s: %w", i.Entity(), err)
		}
		for _, f := range files {
			fi, err := os.Lstat(f.Path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return out, fmt.Errorf("cannot clean %s: %w", f.Path, err)
			}
			if fi.IsDir() {
				continue // a folder is never deleted: only the files an importer lists
			}
			if err := os.Remove(f.Path); err != nil {
				return out, fmt.Errorf("cannot delete %s: %w", f.Path, err)
			}
			res.Deleted++
			res.Bytes += fi.Size()
		}
		out = append(out, res)
	}
	return out, nil
}
