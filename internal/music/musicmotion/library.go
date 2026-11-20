package musicmotion

// library.go: the library of characters and backgrounds of the illustrated style. A character is a folder with a
// manifest (name, size, anchor, poses, expressions) and one SVG per pose; a background is a folder with a manifest
// and one SVG. The library is read on every request from the items that come with the program and from a folder of the
// user, so an item added by putting a folder there shows up on the next list; an item that is not valid is left out
// with one warning. The SVGs draw with the design's colours (var(--bg), var(--fg), var(--accent)). See
// specs/motion-characters.

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
)

//go:embed library
var libraryFS embed.FS

// BuiltinLibrary is the tree of the characters and backgrounds that come with the program.
func BuiltinLibrary() fs.FS {
	sub, err := fs.Sub(libraryFS, "library")
	if err != nil {
		panic(err) // embedded at build time
	}
	return sub
}

// Anchor is the point of a character's drawing that is placed at its position: (0.5, 1) is the middle of its feet.
type Anchor struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// CharacterManifest describes a character.
type CharacterManifest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     int               `json:"version"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	Anchor      Anchor            `json:"anchor"`
	Poses       map[string]string `json:"poses"` // pose name -> svg file
	// Expressions are the names of the groups (class "expr expr-<name>") the drawings hold; one is shown.
	Expressions       []string `json:"expressions"`
	DefaultPose       string   `json:"default_pose"`
	DefaultExpression string   `json:"default_expression"`
}

// Character is a character of the library with its drawings.
type Character struct {
	CharacterManifest
	// SVG holds the drawing of each pose.
	SVG    map[string]string `json:"-"`
	Source string            `json:"source"`
}

// PoseNames lists the poses of the character, sorted.
func (c Character) PoseNames() []string {
	out := make([]string, 0, len(c.Poses))
	for n := range c.Poses {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// HasExpression says whether the character has the expression.
func (c Character) HasExpression(name string) bool {
	for _, e := range c.Expressions {
		if e == name {
			return true
		}
	}
	return false
}

// BackgroundManifest describes a background.
type BackgroundManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	File        string `json:"file"`
}

// Background is a background of the library with its drawing.
type Background struct {
	BackgroundManifest
	SVG    string `json:"-"`
	Source string `json:"source"`
}

// Library reads characters and backgrounds from a built-in tree and a folder of the user. Both have the folders
// characters/ and backgrounds/.
type Library struct {
	Builtin fs.FS
	// UserDir is a folder of the user's items (the folder the library was kept in).
	UserDir string
	// Items is where the user's items are kept; they win over a folder of the same name.
	Items LibraryStore
}

const maxSVGBytes = 200 << 10

var (
	reSVGScript = regexp.MustCompile(`(?i)<script|<foreignObject|\son[a-z]+\s*=`)
	reExtHref   = regexp.MustCompile(`(?i)(?:href|src)\s*=\s*["']\s*(?:https?:)?//|url\(\s*["']?\s*(?:https?:)?//|@import`)
	reExprName  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
)

// svgProblem says what is wrong with a drawing, or "".
func svgProblem(svg string) string {
	switch {
	case len(svg) > maxSVGBytes:
		return fmt.Sprintf("it is larger than %d KB", maxSVGBytes>>10)
	case !strings.Contains(strings.ToLower(svg), "<svg"):
		return "it is not an SVG"
	case reSVGScript.MatchString(svg):
		return "it has a script, a foreignObject or an event handler"
	case reExtHref.MatchString(svg):
		return "it loads something from outside"
	}
	return ""
}

func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func readCharacters(fsys fs.FS, source string) map[string]Character {
	out := map[string]Character{}
	entries, err := fs.ReadDir(fsys, "characters")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		c, bad := checkCharacter(fsys, name, source)
		if bad != "" {
			key := "characters/" + source + "/" + name
			warnOnce(key+"#"+bad, fmt.Sprintf("the character %q is left out: %s", key, bad))
			continue
		}
		out[name] = c
	}
	return out
}

// checkCharacter reads the character in the folder characters/<name> of fsys and checks it: the problem that leaves it out, or "".
func checkCharacter(fsys fs.FS, name, source string) (Character, string) {
	raw, err := fs.ReadFile(fsys, "characters/"+name+"/manifest.json")
	if err != nil {
		return Character{}, "no manifest.json"
	}
	var m CharacterManifest
	if err := decodeStrict(raw, &m); err != nil {
		return Character{}, "manifest.json is not valid: " + err.Error()
	}
	switch {
	case !reName.MatchString(m.Name) || m.Name != name:
		return Character{}, fmt.Sprintf("the name %q must be the name of its folder %q (lower case letters, digits and dashes)", m.Name, name)
	case m.Version < 1 || m.Width <= 0 || m.Height <= 0:
		return Character{}, "the version, width and height must be positive"
	case m.Anchor.X < 0 || m.Anchor.X > 1 || m.Anchor.Y < 0 || m.Anchor.Y > 1:
		return Character{}, "the anchor must be between 0 and 1"
	case len(m.Poses) == 0:
		return Character{}, "it has no poses"
	}
	c := Character{CharacterManifest: m, SVG: map[string]string{}, Source: source}
	for _, pose := range c.PoseNames() { // in order, so that the same problem is said every time
		file := m.Poses[pose]
		if !reExprName.MatchString(pose) {
			return Character{}, fmt.Sprintf("the pose name %q is not valid", pose)
		}
		svg, err := fs.ReadFile(fsys, "characters/"+name+"/"+file)
		if err != nil {
			return Character{}, fmt.Sprintf("the pose %q names the file %q, which is not there", pose, file)
		}
		if p := svgProblem(string(svg)); p != "" {
			return Character{}, fmt.Sprintf("the drawing of the pose %q is refused: %s", pose, p)
		}
		c.SVG[pose] = string(svg)
	}
	if _, ok := m.Poses[m.DefaultPose]; !ok {
		return Character{}, fmt.Sprintf("default_pose %q is not one of its poses", m.DefaultPose)
	}
	for _, x := range m.Expressions {
		if !reExprName.MatchString(x) {
			return Character{}, fmt.Sprintf("the expression name %q is not valid", x)
		}
	}
	if len(m.Expressions) > 0 && !c.HasExpression(m.DefaultExpression) {
		return Character{}, fmt.Sprintf("default_expression %q is not one of its expressions", m.DefaultExpression)
	}
	return c, ""
}

func readBackgrounds(fsys fs.FS, source string) map[string]Background {
	out := map[string]Background{}
	entries, err := fs.ReadDir(fsys, "backgrounds")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		bg, bad := checkBackground(fsys, name, source)
		if bad != "" {
			key := "backgrounds/" + source + "/" + name
			warnOnce(key+"#"+bad, fmt.Sprintf("the background %q is left out: %s", key, bad))
			continue
		}
		out[name] = bg
	}
	return out
}

// checkBackground reads the background in the folder backgrounds/<name> of fsys and checks it: the problem that leaves it out, or "".
func checkBackground(fsys fs.FS, name, source string) (Background, string) {
	raw, err := fs.ReadFile(fsys, "backgrounds/"+name+"/manifest.json")
	if err != nil {
		return Background{}, "no manifest.json"
	}
	var m BackgroundManifest
	if err := decodeStrict(raw, &m); err != nil {
		return Background{}, "manifest.json is not valid: " + err.Error()
	}
	if !reName.MatchString(m.Name) || m.Name != name || m.Version < 1 || m.File == "" {
		return Background{}, "the name must be the name of its folder, the version 1 or more and the file given"
	}
	svg, err := fs.ReadFile(fsys, "backgrounds/"+name+"/"+m.File)
	if err != nil {
		return Background{}, fmt.Sprintf("the file %q is not there", m.File)
	}
	if p := svgProblem(string(svg)); p != "" {
		return Background{}, "the drawing is refused: " + p
	}
	return Background{BackgroundManifest: m, SVG: string(svg), Source: source}, ""
}

func (l Library) trees() []struct {
	fsys   fs.FS
	source string
} {
	var t []struct {
		fsys   fs.FS
		source string
	}
	if l.Builtin != nil {
		t = append(t, struct {
			fsys   fs.FS
			source string
		}{l.Builtin, "builtin"})
	}
	if l.UserDir != "" {
		if st, err := os.Stat(l.UserDir); err == nil && st.IsDir() {
			t = append(t, struct {
				fsys   fs.FS
				source string
			}{os.DirFS(l.UserDir), "user"})
		}
	}
	if l.Items != nil {
		t = append(t, struct {
			fsys   fs.FS
			source string
		}{l.Items.Tree(), "user"})
	}
	return t
}

// Characters lists the valid characters by name; one of the user's replaces a built-in one of the same name.
func (l Library) Characters() []Character {
	all := map[string]Character{}
	for _, t := range l.trees() {
		for n, c := range readCharacters(t.fsys, t.source) {
			all[n] = c
		}
	}
	out := make([]Character, 0, len(all))
	for _, c := range all {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Backgrounds lists the valid backgrounds by name.
func (l Library) Backgrounds() []Background {
	all := map[string]Background{}
	for _, t := range l.trees() {
		for n, b := range readBackgrounds(t.fsys, t.source) {
			all[n] = b
		}
	}
	out := make([]Background, 0, len(all))
	for _, b := range all {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Character returns the character called name.
func (l Library) Character(name string) (Character, bool) {
	for _, c := range l.Characters() {
		if c.Name == name {
			return c, true
		}
	}
	return Character{}, false
}

// Background returns the background called name.
func (l Library) Background(name string) (Background, bool) {
	for _, b := range l.Backgrounds() {
		if b.Name == name {
			return b, true
		}
	}
	return Background{}, false
}
