// Package fsutil holds the small file helpers that several packages need: copying a file, asking whether a path
// exists and the median of some numbers. One copy of each, so they all behave the same.
package fsutil

import (
	"io"
	"os"
	"path/filepath"
	"sort"
)

// CopyFile copies the file src to dst: the folder of dst is created, the data goes to a temporary file next to dst and
// is renamed into place (so a failure, or another copy of the same file at the same time, never leaves dst half
// written), and dst gets the permissions of src.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(dir, filepath.Base(dst)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, fi.Mode().Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Exists reports whether something (a file, a folder, a link) is at path.
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Median is the middle value of v (the mean of the two middle ones when there is an even number); 0 for no values.
// v is not changed.
func Median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}
