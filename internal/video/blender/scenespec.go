package blender

// scenespec.go: the scene description (contract v1, spec blender-engine). A description is a .scene.json file of the
// project's sources: the world, the camera, the lights, the materials and the objects of a 3D scene with keyframes in
// seconds or in beats. Load reads it as data and checks every rule of the contract before Blender starts, so a fault is
// named by its JSON path (objects[3].keys[2].t) and not by a Python traceback. What it returns is the RESOLVED
// description: every key in seconds (a beat converted with the project's tempo) and every colour #rrggbb. That is what
// the script reads and what the cache hashes.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// The limits of contract v1.
const (
	SpecVersion     = 1
	MaxSpecBytes    = 256 << 10
	MaxObjects      = 64
	MaxLights       = 8
	MaxMaterials    = 32
	MaxKeys         = 400
	MaxCoordinate   = 1000.0
	MaxSize         = 1000.0
	MaxTextRunes    = 120
	maxSpecProblems = 20
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Palette is the three colours of the project's design that a description can name: bg, fg and accent.
type Palette struct {
	BG     string `json:"bg"`
	FG     string `json:"fg"`
	Accent string `json:"accent"`
}

// DefaultPalette is what a project without a design gets.
var DefaultPalette = Palette{BG: "#0b0f14", FG: "#e8f1ec", Accent: "#2bff88"}

// PaletteFromDesign makes the palette of a design (its two tones and its accent); a colour that is empty or not
// #rrggbb or #rgb is taken from DefaultPalette.
func PaletteFromDesign(dark, light, accent string) Palette {
	pick := func(c, def string) string {
		if n, ok := normaliseHex(c); ok {
			return n
		}
		return def
	}
	return Palette{BG: pick(dark, DefaultPalette.BG), FG: pick(light, DefaultPalette.FG), Accent: pick(accent, DefaultPalette.Accent)}
}

func normaliseHex(c string) (string, bool) {
	c = strings.ToLower(strings.TrimSpace(c))
	if len(c) == 4 && c[0] == '#' {
		c = "#" + strings.Repeat(string(c[1]), 2) + strings.Repeat(string(c[2]), 2) + strings.Repeat(string(c[3]), 2)
	}
	if !hexColour.MatchString(c) {
		return "", false
	}
	return c, true
}

// Colour resolves a colour of a description: #rrggbb, or bg, fg or accent from the palette.
func (p Palette) Colour(v string) (string, bool) {
	switch v {
	case "bg":
		return p.BG, true
	case "fg":
		return p.FG, true
	case "accent":
		return p.Accent, true
	}
	if hexColour.MatchString(v) {
		return strings.ToLower(v), true
	}
	return "", false
}

// Env is what a description is checked and resolved against: the project's design and tempo, and its sources folder.
type Env struct {
	// Palette gives the colours bg, fg and accent.
	Palette Palette
	// Sources is the sources folder, where the .ttf and .otf fonts a text names must be ("" skips that check).
	Sources string
	// BPM is the project's tempo (defaults.motion.bpm), 0 when it has none: a key in beats is refused then.
	BPM float64
	// DesignFonts says which of the roles "display" and "body" the project's design has a font for in the server's pack.
	DesignFonts map[string]bool
}

// FontRoles are the two names a text's font can take from the project's design.
const (
	FontDisplay = "display"
	FontBody    = "body"
)

// IsFontRole reports whether a font name is a role of the design and not a file of the sources.
func IsFontRole(name string) bool { return name == FontDisplay || name == FontBody }

// Spec is a scene description.
type Spec struct {
	Version   int                     `json:"version"`
	World     *WorldSpec              `json:"world,omitempty"`
	Camera    *CameraSpec             `json:"camera"`
	Lights    []LightSpec             `json:"lights,omitempty"`
	Materials map[string]MaterialSpec `json:"materials,omitempty"`
	Objects   []ObjectSpec            `json:"objects,omitempty"`
}

// WorldSpec is the background of the scene.
type WorldSpec struct {
	Color    string   `json:"color,omitempty"`
	Strength *float64 `json:"strength,omitempty"`
}

// CameraSpec is the camera: a lens and the keys of its path.
type CameraSpec struct {
	Lens *float64 `json:"lens,omitempty"`
	Keys []Key    `json:"keys"`
}

// LightSpec is one light.
type LightSpec struct {
	Type     string   `json:"type"`
	Location *Vec3    `json:"location,omitempty"`
	Rotation *Vec3    `json:"rotation,omitempty"`
	Target   *Vec3    `json:"target,omitempty"`
	Power    *float64 `json:"power,omitempty"`
	Size     *float64 `json:"size,omitempty"`
	Color    string   `json:"color,omitempty"`
}

// MaterialSpec is one named material.
type MaterialSpec struct {
	Color     string   `json:"color,omitempty"`
	Roughness *float64 `json:"roughness,omitempty"`
	Metallic  *float64 `json:"metallic,omitempty"`
	Emission  *float64 `json:"emission,omitempty"`
}

// ObjectSpec is one object of the scene.
type ObjectSpec struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Size     *float64 `json:"size,omitempty"`
	Material string   `json:"material,omitempty"`
	Text     string   `json:"text,omitempty"`
	Extrude  *float64 `json:"extrude,omitempty"`
	Bevel    *float64 `json:"bevel,omitempty"`
	Align    string   `json:"align,omitempty"`
	Font     string   `json:"font,omitempty"`
	// Curve is the radius, in metres, of the quarter circle that joins the floor and the wall of a backdrop.
	Curve *float64 `json:"curve,omitempty"`
	Keys  []Key    `json:"keys,omitempty"`
}

// Key is a keyframe: the channels it gives are keyed at T seconds (or at Beat, converted to T while checking), the
// others are left as the previous key had them.
type Key struct {
	T        *float64 `json:"t,omitempty"`
	Beat     *float64 `json:"beat,omitempty"`
	Location *Vec3    `json:"location,omitempty"`
	Rotation *Vec3    `json:"rotation,omitempty"`
	Target   *Vec3    `json:"target,omitempty"`
	Scale    *Scale   `json:"scale,omitempty"`
	Emission *float64 `json:"emission,omitempty"`
	Ease     string   `json:"ease,omitempty"`
}

// Vec3 is a list of exactly three numbers.
type Vec3 [3]float64

// UnmarshalJSON refuses a list that is not three numbers (a Go array would fill a short one with zeros).
func (v *Vec3) UnmarshalJSON(b []byte) error {
	var l []float64
	if err := json.Unmarshal(b, &l); err != nil || len(l) != 3 {
		return errors.New("expected a list of exactly three numbers")
	}
	copy(v[:], l)
	return nil
}

// Scale is a number (the same on the three axes) or three numbers.
type Scale struct {
	V [3]float64
	// One is true when the description gave a single number.
	One bool
}

// UnmarshalJSON reads a number or a list of three numbers.
func (s *Scale) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		s.V, s.One = [3]float64{n, n, n}, true
		return nil
	}
	var v Vec3
	if err := json.Unmarshal(b, &v); err != nil {
		return errors.New("a scale is a number or a list of three numbers")
	}
	s.V = v
	return nil
}

// MarshalJSON writes it as it was read.
func (s Scale) MarshalJSON() ([]byte, error) {
	if s.One {
		return json.Marshal(s.V[0])
	}
	return json.Marshal(s.V)
}

// The kinds of the contract.
var (
	objectTypes = map[string]bool{"text": true, "cube": true, "sphere": true, "cylinder": true, "cone": true, "torus": true, "plane": true, "backdrop": true}
	lightTypes  = map[string]bool{"sun": true, "area": true, "point": true}
	eases       = map[string]bool{"linear": true, "inout": true, "in": true, "out": true, "back": true, "bounce": true, "elastic": true, "step": true}
	aligns      = map[string]bool{"left": true, "center": true, "right": true}
)

// ObjectTypes is the object types the contract accepts, sorted. The writer's prompt is made from the contract's tables, so
// what a model is told cannot drift from what is accepted.
func ObjectTypes() []string { return sortedNames(objectTypes) }

// LightTypes is the light types the contract accepts, sorted.
func LightTypes() []string { return sortedNames(lightTypes) }

// Eases is the easings the contract accepts, sorted.
func Eases() []string { return sortedNames(eases) }

// Aligns is the alignments of a text the contract accepts, sorted.
func Aligns() []string { return sortedNames(aligns) }

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Problems is what Load returns when a description is not valid: one entry per fault, each starting with its JSON path.
type Problems []string

// Error lists the faults.
func (p Problems) Error() string {
	return "scene description: " + strings.Join(p, "; ")
}

// Load reads and checks the description at path for a scene sceneSeconds long, against env, and returns it resolved.
// The returned error is a Problems when the description breaks a rule of the contract.
func Load(path string, sceneSeconds float64, env Env) (*Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("scene description: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxSpecBytes+1))
	if err != nil {
		return nil, fmt.Errorf("scene description: %w", err)
	}
	if len(data) > MaxSpecBytes {
		return nil, Problems{fmt.Sprintf("the file is larger than %d KiB", MaxSpecBytes>>10)}
	}
	return Parse(data, sceneSeconds, env)
}

// Parse is Load for the bytes of a description.
func Parse(data []byte, sceneSeconds float64, env Env) (*Spec, error) {
	if len(data) > MaxSpecBytes {
		return nil, Problems{fmt.Sprintf("the file is larger than %d KiB", MaxSpecBytes>>10)}
	}
	s, err := Decode(data)
	if err != nil {
		return nil, err
	}
	c := checker{seconds: sceneSeconds, env: env}
	c.check(s)
	if len(c.problems) > 0 {
		if len(c.problems) > maxSpecProblems {
			more := len(c.problems) - maxSpecProblems
			c.problems = append(c.problems[:maxSpecProblems], fmt.Sprintf("and %d more", more))
		}
		return nil, c.problems
	}
	s.resolveColours(env.Palette)
	return s, nil
}

// Resolve reads a description and resolves what can be resolved without checking any rule: the beats of the keys that
// are valid numbers (when env has a tempo) and the colours that are valid. It is what the cache key hashes, so a
// description that Load would refuse (a beat beyond the scene) still gets a key, and the run then fails naming the fault.
func Resolve(data []byte, env Env) (*Spec, error) {
	s, err := Decode(data)
	if err != nil {
		return nil, err
	}
	for _, keys := range s.allKeys() {
		for j := range keys {
			if k := &keys[j]; k.T == nil && k.Beat != nil && env.BPM > 0 && !math.IsNaN(*k.Beat) && !math.IsInf(*k.Beat, 0) {
				t := *k.Beat * 60 / env.BPM
				k.T, k.Beat = &t, nil
			}
		}
	}
	s.resolveColours(env.Palette)
	return s, nil
}

// allKeys is the key lists of the camera and of every object (slices that share their array with the Spec).
func (s *Spec) allKeys() [][]Key {
	var out [][]Key
	if s.Camera != nil {
		out = append(out, s.Camera.Keys)
	}
	for _, o := range s.Objects {
		out = append(out, o.Keys)
	}
	return out
}

// resolveColours writes every colour of the description as #rrggbb, defaults included (the world is bg, a material's
// colour is fg): bg, fg and accent become the palette's, and a colour that is not valid is left as it is (the check
// refused it, or this is the cache key of a description that will fail). A description that names no colour of the
// design is therefore the same whatever the design says.
func (s *Spec) resolveColours(p Palette) {
	fix := func(v, def string) string {
		if v == "" {
			v = def
		}
		if r, ok := p.Colour(v); ok {
			return r
		}
		return v
	}
	if s.World == nil {
		s.World = &WorldSpec{}
	}
	s.World.Color = fix(s.World.Color, "bg")
	for i := range s.Lights {
		s.Lights[i].Color = fix(s.Lights[i].Color, "#ffffff")
	}
	for name, m := range s.Materials {
		m.Color = fix(m.Color, "fg")
		s.Materials[name] = m
	}
}

// JSON is the resolved description as the bytes the script reads and the cache hashes (the order of a Go struct, the
// keys of a map sorted).
func (s *Spec) JSON() ([]byte, error) { return json.Marshal(s) }

// Decode reads the bytes of a description into a Spec without checking any rule of the contract (Parse does that). It
// refuses what is not JSON of the right shape, and fields the contract does not have.
func Decode(data []byte) (*Spec, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		var ute *json.UnmarshalTypeError
		if errors.As(err, &ute) {
			return nil, Problems{fmt.Sprintf("%s: expected %s, got %s", ute.Field, ute.Type.String(), ute.Value)}
		}
		return nil, Problems{"not valid JSON for a description: " + err.Error()}
	}
	if dec.More() {
		return nil, Problems{"there is more than one JSON value in the file"}
	}
	return &s, nil
}

type checker struct {
	seconds  float64
	env      Env
	problems Problems
}

func (c *checker) bad(path, format string, a ...any) {
	c.problems = append(c.problems, path+": "+fmt.Sprintf(format, a...))
}

func (c *checker) colour(path, v string) {
	if v == "" {
		return
	}
	if _, ok := c.env.Palette.Colour(v); !ok {
		c.bad(path, "%q is not #rrggbb, bg, fg or accent", v)
	}
}

func (c *checker) number(path string, v *float64, lo, hi float64) {
	if v == nil {
		return
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < lo || *v > hi {
		c.bad(path, "%v is outside %v to %v", *v, lo, hi)
	}
}

func (c *checker) vec(path string, v *Vec3, limit float64) {
	if v == nil {
		return
	}
	for i, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > limit {
			c.bad(fmt.Sprintf("%s[%d]", path, i), "%v is outside ±%v", x, limit)
		}
	}
}

func (c *checker) check(s *Spec) {
	if s.Version != SpecVersion {
		c.bad("version", "must be %d, got %d", SpecVersion, s.Version)
	}
	if w := s.World; w != nil {
		c.colour("world.color", w.Color)
		c.number("world.strength", w.Strength, 0, 1000)
	}
	if len(s.Materials) > MaxMaterials {
		c.bad("materials", "at most %d materials, got %d", MaxMaterials, len(s.Materials))
	}
	names := make([]string, 0, len(s.Materials))
	for name := range s.Materials {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		m := s.Materials[name]
		p := "materials." + name
		if !idPattern.MatchString(name) {
			c.bad(p, "the name must match %s", idPattern)
		}
		c.colour(p+".color", m.Color)
		c.number(p+".roughness", m.Roughness, 0, 1)
		c.number(p+".metallic", m.Metallic, 0, 1)
		c.number(p+".emission", m.Emission, 0, 10000)
	}
	if cam := s.Camera; cam == nil {
		c.bad("camera", "is required")
	} else {
		if cam.Lens != nil && (*cam.Lens <= 0 || *cam.Lens > 500) {
			c.bad("camera.lens", "%v is outside (0, 500]", *cam.Lens)
		}
		c.keys("camera", cam.Keys, "camera")
	}
	if len(s.Lights) > MaxLights {
		c.bad("lights", "at most %d lights, got %d", MaxLights, len(s.Lights))
	}
	for i, l := range s.Lights {
		p := fmt.Sprintf("lights[%d]", i)
		if !lightTypes[l.Type] {
			c.bad(p+".type", "%q is not sun, area or point", l.Type)
		}
		if l.Target != nil && l.Rotation != nil {
			c.bad(p, "give a target or a rotation, not both")
		}
		if l.Target != nil && l.Type == "sun" {
			c.bad(p+".target", "a sun is aimed with a rotation")
		}
		c.vec(p+".location", l.Location, MaxCoordinate)
		c.vec(p+".target", l.Target, MaxCoordinate)
		c.vec(p+".rotation", l.Rotation, 360*100)
		c.number(p+".power", l.Power, 0, 1e7)
		if l.Size != nil && (*l.Size <= 0 || *l.Size > MaxSize) {
			c.bad(p+".size", "%v is outside (0, %v]", *l.Size, MaxSize)
		}
		c.colour(p+".color", l.Color)
	}
	if len(s.Objects) > MaxObjects {
		c.bad("objects", "at most %d objects, got %d", MaxObjects, len(s.Objects))
	}
	seen := map[string]bool{}
	backdrops := 0
	for i, o := range s.Objects {
		p := fmt.Sprintf("objects[%d]", i)
		switch {
		case !idPattern.MatchString(o.ID):
			c.bad(p+".id", "%q must match %s", o.ID, idPattern)
		case seen[o.ID]:
			c.bad(p+".id", "%q is used twice", o.ID)
		}
		seen[o.ID] = true
		if o.Type == "backdrop" {
			backdrops++
			if backdrops > 1 {
				c.bad(p+".type", "a scene has at most one backdrop")
			}
		}
		if !objectTypes[o.Type] {
			c.bad(p+".type", "%q is not text, cube, sphere, cylinder, cone, torus, plane or backdrop", o.Type)
		}
		switch {
		case o.Size == nil:
			c.bad(p+".size", "is required")
		case *o.Size <= 0 || *o.Size > MaxSize || math.IsNaN(*o.Size):
			c.bad(p+".size", "%v is outside (0, %v]", *o.Size, MaxSize)
		}
		if o.Material != "" {
			if _, ok := s.Materials[o.Material]; !ok {
				c.bad(p+".material", "object %q names the material %q, which the description does not define", o.ID, o.Material)
			}
		}
		switch {
		case o.Type == "backdrop":
			c.backdrop(p, o)
		case o.Curve != nil:
			c.bad(p+".curve", "curve is only for objects of type backdrop")
		}
		if o.Type == "text" {
			c.text(p, o)
		} else if o.Text != "" || o.Extrude != nil || o.Bevel != nil || o.Align != "" || o.Font != "" {
			c.bad(p, "text, extrude, bevel, align and font are only for objects of type text")
		}
		c.keys(p, o.Keys, "object")
	}
}

// backdrop checks the curve of a backdrop: the radius of the quarter circle between the floor and the wall, which has
// to fit in half of its size.
func (c *checker) backdrop(p string, o ObjectSpec) {
	if o.Curve == nil {
		c.bad(p+".curve", "is required for a backdrop")
		return
	}
	if o.Size == nil || *o.Size <= 0 || *o.Size > MaxSize {
		return // the size's own fault is already named
	}
	if math.IsNaN(*o.Curve) || *o.Curve < 0.1 || *o.Curve > *o.Size/2 {
		c.bad(p+".curve", "%v is outside 0.1 to %v (half of the size)", *o.Curve, *o.Size/2)
	}
}

func (c *checker) text(p string, o ObjectSpec) {
	switch {
	case o.Text == "":
		c.bad(p+".text", "is required for a text object")
	case utf8.RuneCountInString(o.Text) > MaxTextRunes:
		c.bad(p+".text", "at most %d characters, got %d", MaxTextRunes, utf8.RuneCountInString(o.Text))
	}
	c.number(p+".extrude", o.Extrude, 0, MaxSize)
	c.number(p+".bevel", o.Bevel, 0, MaxSize)
	if o.Align != "" && !aligns[o.Align] {
		c.bad(p+".align", "%q is not left, center or right", o.Align)
	}
	if o.Font != "" {
		c.font(p+".font", o.Font)
	}
}

// font checks the font of a text: display or body (the project's design, which must have a font in the server's pack),
// or a plain .ttf or .otf name that exists in the sources and stays inside them.
func (c *checker) font(p, name string) {
	if IsFontRole(name) {
		if !c.env.DesignFonts[name] {
			c.bad(p, "the design's %s font is not one of the server's font pack (set a font of the pack in the project's design)", name)
		}
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || name == "." || name == ".." || (ext != ".ttf" && ext != ".otf") {
		c.bad(p, "%q must be display, body, or the name of a .ttf or .otf file of the sources", name)
		return
	}
	if c.env.Sources == "" {
		return
	}
	dir, err := filepath.EvalSymlinks(c.env.Sources)
	if err != nil {
		c.bad(p, "the font %q was not found: the sources folder cannot be read", name)
		return
	}
	file, err := filepath.EvalSymlinks(filepath.Join(c.env.Sources, name))
	if err != nil {
		c.bad(p, "the font %q was not found in the sources", name)
		return
	}
	if rel, err := filepath.Rel(dir, file); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		c.bad(p, "the font %q resolves outside the sources", name)
	}
}

// when checks the time of a key: exactly one of t (seconds) and beat, inside the scene. A beat is converted to seconds
// with the project's tempo and written as the key's t, so from here on the key is in seconds.
func (c *checker) when(p string, k *Key) {
	switch {
	case k.T != nil && k.Beat != nil:
		c.bad(p, "give t (seconds) or beat, not both")
	case k.T == nil && k.Beat == nil:
		c.bad(p, "give t (seconds) or beat")
	case k.T != nil:
		if math.IsNaN(*k.T) || *k.T < 0 || *k.T > c.seconds+1e-9 {
			c.bad(p+".t", "%v is outside the scene (0 to %v s)", *k.T, c.seconds)
		}
	case c.env.BPM <= 0:
		c.bad(p+".beat", "the project has no tempo, so a key cannot be in beats (set defaults.motion.bpm, or use t)")
	case math.IsNaN(*k.Beat) || math.IsInf(*k.Beat, 0) || *k.Beat < 0:
		c.bad(p+".beat", "%v is not a beat from 0", *k.Beat)
	default:
		t := *k.Beat * 60 / c.env.BPM
		if t > c.seconds+1e-9 {
			c.bad(p+".beat", "beat %v falls at %v s, outside the scene (0 to %v s)", *k.Beat, t, c.seconds)
			return
		}
		k.T, k.Beat = &t, nil
	}
}

// keys checks the keyframes of the camera or of an object.
func (c *checker) keys(owner string, keys []Key, kind string) {
	if len(keys) > MaxKeys {
		c.bad(owner+".keys", "at most %d keys, got %d", MaxKeys, len(keys))
		return
	}
	if kind == "camera" && len(keys) == 0 {
		c.bad(owner+".keys", "the camera needs at least one key (its position)")
	}
	for j := range keys {
		k := &keys[j]
		p := fmt.Sprintf("%s.keys[%d]", owner, j)
		c.when(p, k)
		if k.Ease != "" && !eases[k.Ease] {
			c.bad(p+".ease", "%q is not linear, inout, in, out, back, bounce, elastic or step", k.Ease)
		}
		c.vec(p+".location", k.Location, MaxCoordinate)
		c.vec(p+".target", k.Target, MaxCoordinate)
		c.vec(p+".rotation", k.Rotation, 360*100)
		if k.Scale != nil {
			if kind == "camera" {
				c.bad(p+".scale", "a camera has no scale")
			}
			v := Vec3(k.Scale.V)
			c.vec(p+".scale", &v, MaxCoordinate)
		}
		c.number(p+".emission", k.Emission, 0, 10000)
		switch kind {
		case "camera":
			if k.Emission != nil {
				c.bad(p+".emission", "a camera has no emission")
			}
			if k.Target != nil && k.Rotation != nil {
				c.bad(p, "give a target or a rotation, not both")
			}
		default:
			if k.Target != nil {
				c.bad(p+".target", "only the camera and the lights have a target")
			}
		}
	}
}

// Fonts is the names of the font files of the sources the description uses (the roles display and body are not files),
// sorted and without repeats.
func (s *Spec) Fonts() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range s.Objects {
		if o.Font != "" && !IsFontRole(o.Font) && !seen[o.Font] {
			seen[o.Font] = true
			out = append(out, o.Font)
		}
	}
	sort.Strings(out)
	return out
}

// FontRoles is which of display and body the description uses, in that order.
func (s *Spec) FontRoles() []string {
	var out []string
	for _, role := range []string{FontDisplay, FontBody} {
		for _, o := range s.Objects {
			if o.Font == role {
				out = append(out, role)
				break
			}
		}
	}
	return out
}

// FontsHash is one SHA-256 over the content of the font files of the sources the description uses (found in
// sourcesDir), or "" when it uses none.
func (s *Spec) FontsHash(sourcesDir string) string {
	fonts := s.Fonts()
	if len(fonts) == 0 {
		return ""
	}
	h := sha256.New()
	for _, name := range fonts {
		h.Write([]byte(name + "\x00"))
		f, err := os.Open(filepath.Join(sourcesDir, name))
		if err != nil {
			h.Write([]byte("unreadable"))
			continue
		}
		_, _ = io.Copy(h, f)
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil))
}
