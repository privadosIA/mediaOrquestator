//go:build !linux

package workspace

import "errors"

// FreeDiskBytes is unavailable off Linux; callers skip the check.
func FreeDiskBytes(string) (uint64, error) {
	return 0, errors.New("free space unavailable on this platform")
}
