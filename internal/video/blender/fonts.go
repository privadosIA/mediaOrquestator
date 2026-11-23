package blender

// fonts.go: the fonts of the design for the 3D texts. The server's font pack is WOFF2 (what a web page wants) and Blender
// reads TTF and OTF only, so a font is decoded to SFNT once, with a decoder written in Go, and kept in the cache folder
// under the SHA-256 of the WOFF2 it came from. The decoded font is what the script loads and what the .blend packs.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tdewolff/font"
)

// FontSHA is the SHA-256, in hex, of the bytes of a font: the identity of the font in the cache key and in the cache folder.
func FontSHA(woff2 []byte) string {
	sum := sha256.Sum256(woff2)
	return hex.EncodeToString(sum[:])
}

// EnsureTTF decodes a WOFF2 font to a TrueType file in dir, named by the SHA-256 of the WOFF2, and returns its path. A
// font already decoded is reused without decoding it again. The file is written to a temporary name and renamed, so a
// run that stops half way leaves no broken font behind.
func EnsureTTF(dir string, woff2 []byte) (string, error) {
	path := filepath.Join(dir, FontSHA(woff2)+".ttf")
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return path, nil
	}
	sfnt, err := font.ParseWOFF2(woff2)
	if err != nil {
		return "", fmt.Errorf("blender: cannot decode the font: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "font-*.tmp")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(sfnt)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return "", werr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}
