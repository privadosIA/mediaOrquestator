package music

import (
	"os"
	"syscall"
)

// deviceOf is the number of the file system a path is on (0 when it cannot be read), so that the disk an install needs is added up per disk.
func deviceOf(path string) uint64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}
