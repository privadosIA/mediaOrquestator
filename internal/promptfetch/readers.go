package promptfetch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ReadCSV streams the rows of a CSV file with only the named columns. Every other column is scanned past and never
// stored: its text is not kept, not even for the length of a row. Quoted fields, doubled quotes, newlines inside quotes
// and a BOM are handled; the values are strings.
func ReadCSV(path string, columns []string, fn func(Row) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	if b, err := r.Peek(3); err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = r.Discard(3)
	}
	wanted := map[string]bool{}
	for _, c := range columns {
		wanted[c] = true
	}
	var (
		header []string
		keep   []bool
		vals   []string // one entry per field of the row; "" for a column that is skipped
		cur    []byte
		quoted bool
		inRow  bool
	)
	keepCol := func(i int) bool { return header == nil || (i < len(keep) && keep[i]) }
	endField := func() {
		if keepCol(len(vals)) {
			vals = append(vals, string(cur))
		} else {
			vals = append(vals, "")
		}
		cur = cur[:0]
	}
	endRow := func() error {
		defer func() { vals, cur, inRow = vals[:0], cur[:0], false }()
		if !inRow {
			return nil // an empty line
		}
		endField()
		if header == nil {
			header = append([]string(nil), vals...)
			keep = make([]bool, len(header))
			for i, h := range header {
				keep[i] = wanted[strings.TrimSpace(h)]
				header[i] = strings.TrimSpace(h)
			}
			for _, c := range columns {
				if !slices.Contains(header, c) {
					return fmt.Errorf("%s has no column %q", path, c)
				}
			}
			return nil
		}
		row := Row{}
		for i, h := range header {
			if keep[i] && i < len(vals) {
				row[h] = vals[i]
			}
		}
		return fn(row)
	}
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if quoted || (b != '\n' && b != '\r') {
			inRow = true // an empty line is not a row
		}
		keepField := keepCol(len(vals))
		switch {
		case quoted:
			if b == '"' {
				if nb, err := r.Peek(1); err == nil && nb[0] == '"' {
					_, _ = r.ReadByte()
					if keepField {
						cur = append(cur, '"')
					}
				} else {
					quoted = false
				}
			} else if keepField {
				cur = append(cur, b)
			}
		case b == '"':
			quoted = true
		case b == ',':
			endField()
		case b == '\n':
			if err := endRow(); err != nil {
				return err
			}
		case b == '\r':
		default:
			if keepField {
				cur = append(cur, b)
			}
		}
	}
	return endRow()
}

// ReadJSON reads a JSON file (a list, or an object with the list under data, items or rows) or JSON lines, with only
// the named columns.
func ReadJSON(path string, columns []string, fn func(Row) error) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var list []map[string]json.RawMessage
	var obj struct {
		Data  []map[string]json.RawMessage `json:"data"`
		Items []map[string]json.RawMessage `json:"items"`
		Rows  []map[string]json.RawMessage `json:"rows"`
	}
	switch {
	case json.Unmarshal(b, &list) == nil:
	case json.Unmarshal(b, &obj) == nil && (obj.Data != nil || obj.Items != nil || obj.Rows != nil):
		list = append(append(obj.Data, obj.Items...), obj.Rows...)
	default:
		dec := json.NewDecoder(bytes.NewReader(b))
		for dec.More() {
			var m map[string]json.RawMessage
			if err := dec.Decode(&m); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			list = append(list, m)
		}
	}
	for _, m := range list {
		row := Row{}
		for _, c := range columns {
			if raw, ok := m[c]; ok {
				var v any
				if json.Unmarshal(raw, &v) == nil {
					row[c] = v
				}
			}
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return nil
}

// ReadFile streams the rows of a downloaded file in the format the source says.
func ReadFile(format, path string, columns []string, fn func(Row) error) error {
	switch format {
	case "parquet":
		return ReadParquet(path, columns, fn)
	case "csv":
		return ReadCSV(path, columns, fn)
	default:
		return ReadJSON(path, columns, fn)
	}
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
