package musicmotion

import (
	"embed"
	"io/fs"
)

//go:embed parts
var partsFS embed.FS

// BuiltinParts is the tree of the parts that come with the program (a folder per part).
func BuiltinParts() fs.FS {
	sub, err := fs.Sub(partsFS, "parts")
	if err != nil {
		panic(err) // the folder is embedded at build time
	}
	return sub
}
