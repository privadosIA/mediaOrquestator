//go:build linux

package workspace

import "syscall"

// FreeDiskBytes reports the bytes available to unprivileged users on the
// filesystem holding dir.
func FreeDiskBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
