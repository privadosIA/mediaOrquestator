//go:build !linux

package music

// deviceOf is 0 where the file system of a path cannot be told: the install then adds everything up against the least free place.
func deviceOf(string) uint64 { return 0 }
