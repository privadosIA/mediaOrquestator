package hyperframes

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
	"sync"
)

//go:embed fonts/*
var fontFiles embed.FS

var fontIndex struct {
	once  sync.Once
	byURL map[string]string // address -> file name in fonts/
}

func loadFontIndex() {
	fontIndex.byURL = map[string]string{}
	b, err := fontFiles.ReadFile("fonts/index.txt")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && !strings.HasPrefix(f[0], "#") {
			fontIndex.byURL[f[1]] = f[0]
		}
	}
}

// FontCSS is the stylesheet kept for a Google Fonts address, or false. The faces in it point at fonts.gstatic.com files that
// the libraries manifest pins and the drawer serves locally.
func FontCSS(rawURL string) ([]byte, bool) {
	fontIndex.once.Do(loadFontIndex)
	name, ok := fontIndex.byURL[rawURL]
	if !ok {
		return nil, false
	}
	b, err := fontFiles.ReadFile("fonts/" + name + ".css")
	return b, err == nil
}

// FontCSSName is the file name FontCSS keeps an address under (the first 16 hex digits of its SHA-256).
func FontCSSName(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(h[:])[:16]
}
