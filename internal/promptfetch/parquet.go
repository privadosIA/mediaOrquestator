package promptfetch

import (
	"fmt"
	"io"
	"os"

	"github.com/parquet-go/parquet-go"
)

// Row is one row of a dataset with only the columns a source names: a string, a float64, an int64, a bool, or nil.
type Row map[string]any

const valueBatch = 1024

// columnStream walks the values of one column of a row group, a page at a time.
type columnStream struct {
	pages parquet.Pages
	vals  parquet.ValueReader
	buf   []parquet.Value
	n, at int
}

func (c *columnStream) next() (parquet.Value, error) {
	for c.at >= c.n {
		if c.vals == nil {
			p, err := c.pages.ReadPage()
			if err != nil {
				return parquet.Value{}, err
			}
			c.vals = p.Values()
		}
		n, err := c.vals.ReadValues(c.buf)
		c.n, c.at = n, 0
		if n == 0 {
			if err != nil && err != io.EOF {
				return parquet.Value{}, err
			}
			c.vals = nil
		}
	}
	v := c.buf[c.at]
	c.at++
	return v, nil
}

func plain(v parquet.Value) any {
	if v.IsNull() {
		return nil
	}
	switch v.Kind() {
	case parquet.Boolean:
		return v.Boolean()
	case parquet.Int32:
		return int64(v.Int32())
	case parquet.Int64:
		return v.Int64()
	case parquet.Float:
		return float64(v.Float())
	case parquet.Double:
		return v.Double()
	case parquet.ByteArray, parquet.FixedLenByteArray:
		return string(v.ByteArray())
	}
	return nil
}

// ReadParquet streams the rows of a parquet file, one at a time, with only the named columns. A column that is not named
// is never opened: for each row group only the pages of the named columns are requested, so what a lyrics or an image
// column holds is never read from the disk, let alone decoded. A named column the file does not have is an error.
func ReadParquet(path string, columns []string, fn func(Row) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	idx := make([]int, len(columns))
	for i, name := range columns {
		leaf, ok := pf.Schema().Lookup(name)
		if !ok {
			return fmt.Errorf("%s has no column %q", path, name)
		}
		idx[i] = leaf.ColumnIndex
	}
	for _, rg := range pf.RowGroups() {
		chunks := rg.ColumnChunks()
		streams := make([]*columnStream, len(columns))
		for i := range columns {
			streams[i] = &columnStream{pages: chunks[idx[i]].Pages(), buf: make([]parquet.Value, valueBatch)}
		}
		err := func() error {
			defer func() {
				for _, s := range streams {
					_ = s.pages.Close()
				}
			}()
			for r := int64(0); r < rg.NumRows(); r++ {
				row := make(Row, len(columns))
				for i, name := range columns {
					v, err := streams[i].next()
					if err != nil {
						return fmt.Errorf("%s: column %q, row %d: %w", path, name, r, err)
					}
					row[name] = plain(v)
				}
				if err := fn(row); err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
