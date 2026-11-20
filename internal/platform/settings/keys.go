package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// maxKeyLen is the longest API key accepted.
const maxKeyLen = 500

// KeysPath is the file that holds the providers' API keys, next to settings.json.
// It is the only place a key is ever written: never settings.json, scenes.yaml, an
// export, a log or an API answer.
func KeysPath() string {
	d := Dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "keys.json")
}

func loadKeys() map[string]string {
	keys := map[string]string{}
	p := KeysPath()
	if p == "" {
		return keys
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return keys
	}
	if json.Unmarshal(data, &keys) != nil || keys == nil {
		return map[string]string{}
	}
	return keys
}

func saveKeys(keys map[string]string) error {
	p := KeysPath()
	if p == "" {
		return errors.New("settings: cannot find the config directory")
	}
	if len(keys) == 0 {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(p, append(data, '\n'))
}

// ValidateKey checks an API key: at most 500 characters, no spaces, no control characters.
func ValidateKey(key string) error {
	switch {
	case key == "":
		return errors.New("the key is empty")
	case len(key) > maxKeyLen:
		return errors.New("the key is longer than 500 characters")
	case strings.IndexFunc(key, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0:
		return errors.New("the key must have no spaces or control characters")
	}
	return nil
}

// keysMu makes the read-modify-write of the key store take turns, whoever does it (SetKey, DeleteKey and SyncKeys). It is taken after mu, never before.
var keysMu sync.Mutex

// SyncKeys makes the entries of the key store named in values what values says: a non-empty value is stored (without the checks of ValidateKey: it is
// a secret that was already in use), an empty one removes the entry, and every other entry is left alone. The file is not written when nothing
// changes. A Source that keeps no secret in its database uses it to put the secrets of the settings where they belong.
func SyncKeys(values map[string]string) error {
	keysMu.Lock()
	defer keysMu.Unlock()
	keys := loadKeys()
	changed := false
	for id, v := range values {
		if v == "" {
			if _, ok := keys[id]; ok {
				delete(keys, id)
				changed = true
			}
		} else if keys[id] != v {
			keys[id] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return saveKeys(keys)
}

// SetKey stores the key of a provider.
func SetKey(id, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	keysMu.Lock()
	defer keysMu.Unlock()
	keys := loadKeys()
	keys[id] = key
	return saveKeys(keys)
}

// DeleteKey removes the key of a provider; no key is not an error.
func DeleteKey(id string) error {
	mu.Lock()
	defer mu.Unlock()
	return deleteKeyLocked(id)
}

func deleteKeyLocked(id string) error {
	keysMu.Lock()
	defer keysMu.Unlock()
	keys := loadKeys()
	if _, ok := keys[id]; !ok {
		return nil
	}
	delete(keys, id)
	return saveKeys(keys)
}

// Key returns the key of a provider, or "". Only the code that calls a provider
// should use it; everything that reports must use HasKey.
func Key(id string) string { return loadKeys()[id] }

// HasKey says whether a provider has a key, without revealing it.
func HasKey(id string) bool { return loadKeys()[id] != "" }
