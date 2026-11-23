// Package fonts is the pack of free fonts that comes with the program (spec motion-house-style): the WOFF2 files of this
// folder (SIL Open Font License 1.1; the licence of each is next to it), the family each one is and the names a design
// uses for them. It has no dependencies, so both the pages of the music motion mode (which embed the files as data: URLs)
// and the 3D scenes of the pipeline (which decode them to TrueType for Blender) can read it.
package fonts

import (
	"embed"
	"fmt"
)

//go:embed *.woff2
var files embed.FS

// Font is a font of the pack: the family name CSS uses, the weights embedded and the CSS stack that starts with it.
type Font struct {
	Family  string
	Weights []int
	Stack   string
}

// Packed are the fonts of the pack, by the name a design uses.
var Packed = map[string]Font{
	"inter":            {"Inter", []int{400, 700}, `"Inter", "Helvetica Neue", Arial, "Liberation Sans", sans-serif`},
	"space-grotesk":    {"Space Grotesk", []int{400, 700}, `"Space Grotesk", "Helvetica Neue", Arial, "Liberation Sans", sans-serif`},
	"jetbrains-mono":   {"JetBrains Mono", []int{400, 700}, `"JetBrains Mono", "DejaVu Sans Mono", "Liberation Mono", Menlo, Consolas, monospace`},
	"playfair-display": {"Playfair Display", []int{400, 700}, `"Playfair Display", Georgia, "Times New Roman", "Liberation Serif", serif`},
	"bebas-neue":       {"Bebas Neue", []int{400}, `"Bebas Neue", Impact, "Arial Narrow", "Liberation Sans Narrow", sans-serif`},
	"fraunces":         {"Fraunces", []int{400, 700}, `"Fraunces", Georgia, "Times New Roman", "Liberation Serif", serif`},
}

// Alias maps the six stack names that existed before the pack to the font of the pack that replaces their first choice;
// the stack that was theirs stays after it as the fallback.
var Alias = map[string]string{"grotesk": "inter", "mono": "jetbrains-mono", "serif": "fraunces", "heavy": "bebas-neue"}

// Of is the font of the pack a name stands for, if it stands for one.
func Of(name string) (string, bool) {
	if _, ok := Packed[name]; ok {
		return name, true
	}
	p, ok := Alias[name]
	return p, ok
}

// ReadFile is the WOFF2 of a font of the pack (by its own name, not an alias) at a weight.
func ReadFile(pack string, weight int) ([]byte, error) {
	return files.ReadFile(fmt.Sprintf("%s-%d.woff2", pack, weight))
}

// Face is one file of the pack: the family, the weight it is and its WOFF2 bytes.
type Face struct {
	Family string
	Weight int
	WOFF2  []byte
}

// Pick is the file a design's font name stands for, at the weight asked for or at 400 when the font has no such weight.
// ok is false for a name that stands for no font of the pack (a system stack), which has no file to give.
func Pick(name string, weight int) (Face, bool) {
	pack, ok := Of(name)
	if !ok {
		return Face{}, false
	}
	p := Packed[pack]
	w := 400
	for _, have := range p.Weights {
		if have == weight {
			w = weight
		}
	}
	raw, err := ReadFile(pack, w)
	if err != nil {
		return Face{}, false
	}
	return Face{Family: p.Family, Weight: w, WOFF2: raw}, true
}
