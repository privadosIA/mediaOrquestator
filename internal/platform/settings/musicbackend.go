package settings

import (
	"fmt"
	"strings"
)

// The backends of a music generator on this machine (the same names as musicsvc.BackendDotnet and musicsvc.BackendDocker).
const (
	MusicBackendDotnet = "dotnet"
	MusicBackendDocker = "docker"
)

// ValidateMusicBackend returns the backend named by the text ("dotnet" or "docker", in any case), or an error.
func ValidateMusicBackend(raw string) (string, error) {
	switch b := strings.ToLower(strings.TrimSpace(raw)); b {
	case MusicBackendDotnet, MusicBackendDocker:
		return b, nil
	default:
		return "", fmt.Errorf("the backend must be %q or %q", MusicBackendDotnet, MusicBackendDocker)
	}
}

// MusicBackend is how the generator ("heartmula" or "diffrhythm2") runs on this machine: the saved backend, else the .NET service.
// An external address saved for the generator wins over it (the caller checks that first).
func MusicBackend(engine string) string {
	f := Load()
	saved := f.HeartMulaBackend
	if engine == "diffrhythm2" {
		saved = f.DiffRhythm2Backend
	}
	if saved == MusicBackendDocker {
		return MusicBackendDocker
	}
	return MusicBackendDotnet
}

// MusicBackendSource says where MusicBackend gets the backend from: FromSetting when one is saved, FromDefault otherwise.
func MusicBackendSource(engine string) string {
	f := Load()
	saved := f.HeartMulaBackend
	if engine == "diffrhythm2" {
		saved = f.DiffRhythm2Backend
	}
	if saved != "" {
		return FromSetting
	}
	return FromDefault
}

// SetMusicBackend saves the backend of a generator (already validated); an empty one removes the setting.
func SetMusicBackend(engine, backend string) error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if engine == "diffrhythm2" {
		f.DiffRhythm2Backend = backend
	} else {
		f.HeartMulaBackend = backend
	}
	return Save(f)
}
