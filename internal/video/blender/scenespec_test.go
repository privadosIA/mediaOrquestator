package blender

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleSpec = "../../../testdata/blender-build/sources/titulo.scene.json"

var testPalette = Palette{BG: "#0b0f14", FG: "#e8f1ec", Accent: "#ff3b30"}

// minimal is the smallest valid description: a camera with one key and nothing else.
const minimal = `{"version":1,"camera":{"keys":[{"t":0,"location":[0,-5,1],"target":[0,0,0]}]}}`

// testEnv is a project with the example's palette, a tempo of 120 bpm and a design with both fonts of the pack.
func testEnv(sources string) Env {
	return Env{Palette: testPalette, Sources: sources, BPM: 120, DesignFonts: map[string]bool{FontDisplay: true, FontBody: true}}
}

func parse(t *testing.T, doc string) error {
	t.Helper()
	_, err := Parse([]byte(doc), 5, testEnv(t.TempDir()))
	return err
}

func TestSpec_TheExampleIsValid(t *testing.T) {
	s, err := Load(exampleSpec, 5, testEnv(filepath.Dir(exampleSpec)))
	if err != nil {
		t.Fatalf("the example: %v", err)
	}
	if len(s.Objects) != 3 || len(s.Lights) != 2 || len(s.Materials) != 3 {
		t.Errorf("objects %d, lights %d, materials %d", len(s.Objects), len(s.Lights), len(s.Materials))
	}
	if s.World.Color != "#0b0f14" || s.Materials["pulse"].Color != "#ff3b30" || s.Materials["title"].Color != "#e8f1ec" {
		t.Errorf("the colours must come out resolved: world %q, pulse %q, title %q", s.World.Color, s.Materials["pulse"].Color, s.Materials["title"].Color)
	}
	for _, o := range s.Objects {
		for i, k := range o.Keys {
			if k.T == nil || k.Beat != nil {
				t.Errorf("%s.keys[%d] must come out in seconds only: %+v", o.ID, i, k)
			}
		}
	}
}

func TestSpec_AKeyAtTheLengthOfTheSceneIsAccepted(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0,"location":[0,-5,1]},{"t":5,"location":[0,-4,1]}]}}`
	if err := parse(t, doc); err != nil {
		t.Errorf("t equal to the length: %v", err)
	}
}

func TestSpec_Refusals(t *testing.T) {
	obj := func(extra string) string {
		return `{"version":1,"camera":{"keys":[{"t":0,"location":[0,-5,1]}]},"materials":{"m":{"color":"fg"}},"objects":[{"id":"a","type":"cube","size":1,` + extra + `}]}`
	}
	many := func(n int, item string) string { return strings.TrimSuffix(strings.Repeat(item+",", n), ",") }
	for name, tc := range map[string]struct{ doc, want string }{
		"not json":            {`{`, "not valid JSON"},
		"two values":          {minimal + minimal, "more than one JSON value"},
		"unknown field":       {`{"version":1,"camera":{"keys":[]},"fog":1}`, `unknown field "fog"`},
		"wrong type":          {`{"version":"1"}`, "version: expected int"},
		"version":             {`{"version":2,"camera":{"keys":[{"t":0}]}}`, "version: must be 1"},
		"no camera":           {`{"version":1}`, "camera: is required"},
		"camera without keys": {`{"version":1,"camera":{"keys":[]}}`, "camera.keys: the camera needs at least one key"},
		"lens":                {`{"version":1,"camera":{"lens":0,"keys":[{"t":0}]}}`, "camera.lens"},
		"world colour":        {`{"version":1,"world":{"color":"red"},"camera":{"keys":[{"t":0}]}}`, "world.color"},
		"a key out of range":  {`{"version":1,"camera":{"keys":[{"t":0},{"t":1},{"t":2},{"t":7}]}}`, "camera.keys[3].t: 7 is outside the scene (0 to 5 s)"},
		"a negative t":        {`{"version":1,"camera":{"keys":[{"t":-1}]}}`, "camera.keys[0].t"},
		"a key without t":     {`{"version":1,"camera":{"keys":[{"location":[0,0,0]}]}}`, "camera.keys[0]: give t (seconds) or beat"},
		"ease":                {`{"version":1,"camera":{"keys":[{"t":0,"ease":"wobble"}]}}`, "camera.keys[0].ease"},
		"coordinate":          {`{"version":1,"camera":{"keys":[{"t":0,"location":[0,1001,0]}]}}`, "camera.keys[0].location[1]"},
		"camera scale":        {`{"version":1,"camera":{"keys":[{"t":0,"scale":2}]}}`, "a camera has no scale"},
		"camera emission":     {`{"version":1,"camera":{"keys":[{"t":0,"emission":2}]}}`, "a camera has no emission"},
		"target and rotation": {`{"version":1,"camera":{"keys":[{"t":0,"target":[0,0,0],"rotation":[0,0,0]}]}}`, "a target or a rotation"},
		"object target":       {obj(`"keys":[{"t":0,"target":[0,0,0]}]`), "only the camera and the lights have a target"},
		"too many keys":       {`{"version":1,"camera":{"keys":[` + many(401, `{"t":0}`) + `]}}`, "at most 400 keys"},
		"too many objects":    {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[` + many(65, `{"id":"a","type":"cube","size":1}`) + `]}`, "at most 64 objects"},
		"too many lights":     {`{"version":1,"camera":{"keys":[{"t":0}]},"lights":[` + many(9, `{"type":"sun"}`) + `]}`, "at most 8 lights"},
		"light type":          {`{"version":1,"camera":{"keys":[{"t":0}]},"lights":[{"type":"spot"}]}`, "lights[0].type"},
		"sun target":          {`{"version":1,"camera":{"keys":[{"t":0}]},"lights":[{"type":"sun","target":[0,0,0]}]}`, "lights[0].target"},
		"light colour":        {`{"version":1,"camera":{"keys":[{"t":0}]},"lights":[{"type":"point","color":"blue"}]}`, "lights[0].color"},
		"bad id":              {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"A b","type":"cube","size":1}]}`, "objects[0].id"},
		"repeated id":         {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":1},{"id":"a","type":"cube","size":1}]}`, `objects[1].id: "a" is used twice`},
		"object type":         {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"monkey","size":1}]}`, "objects[0].type"},
		"size missing":        {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube"}]}`, "objects[0].size: is required"},
		"size zero":           {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":0}]}`, "objects[0].size"},
		"size huge":           {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":1001}]}`, "objects[0].size"},
		"unknown material":    {obj(`"material":"nope"`), `object "a" names the material "nope"`},
		"material name":       {`{"version":1,"camera":{"keys":[{"t":0}]},"materials":{"Bad Name":{}}}`, "materials.Bad Name"},
		"material colour":     {`{"version":1,"camera":{"keys":[{"t":0}]},"materials":{"m":{"color":"#12"}}}`, "materials.m.color"},
		"roughness":           {`{"version":1,"camera":{"keys":[{"t":0}]},"materials":{"m":{"roughness":2}}}`, "materials.m.roughness"},
		"text only for text":  {obj(`"text":"hi"`), "only for objects of type text"},
		"text missing":        {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1}]}`, "objects[0].text: is required"},
		"text too long":       {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"` + strings.Repeat("a", 121) + `"}]}`, "objects[0].text: at most 120"},
		"align":               {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","align":"top"}]}`, "objects[0].align"},
		"font is a path":      {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"../f.ttf"}]}`, "objects[0].font"},
		"font absolute":       {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"/etc/f.ttf"}]}`, "objects[0].font"},
		"font not a font":     {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"f.woff2"}]}`, "objects[0].font"},
		"font missing":        {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"gone.ttf"}]}`, `font "gone.ttf" was not found`},
		"a location of two":   {`{"version":1,"camera":{"keys":[{"t":0,"location":[1,2]}]}}`, "exactly three numbers"},
		"a scale of two":      {obj(`"keys":[{"t":0,"scale":[1,2]}]`), "scale"},
	} {
		t.Run(name, func(t *testing.T) {
			err := parse(t, tc.doc)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSpec_AnObjectIsRefusedBeforeBlenderStartsNamingThePath(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[
	  {"id":"a","type":"cube","size":1},{"id":"b","type":"cube","size":1},{"id":"c","type":"cube","size":1},
	  {"id":"d","type":"cube","size":1,"keys":[{"t":0},{"t":1},{"t":9}]}]}`
	err := parse(t, doc)
	if _, ok := err.(Problems); !ok || !strings.Contains(err.Error(), "objects[3].keys[2].t") {
		t.Errorf("err = %v (%T)", err, err)
	}
}

func TestSpec_TooLargeAFileIsRefusedUnread(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.scene.json")
	os.WriteFile(path, []byte(`{"version":1,"pad":"`+strings.Repeat("x", MaxSpecBytes)+`"}`), 0o644)
	_, err := Load(path, 5, testEnv(dir))
	if err == nil || !strings.Contains(err.Error(), "larger than 256 KiB") {
		t.Errorf("err = %v", err)
	}
}

func TestSpec_AFontOfTheSourcesIsFoundAndOneOutsideIsNot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.ttf"), []byte("font"), 0o644)
	outside := filepath.Join(t.TempDir(), "o.ttf")
	os.WriteFile(outside, []byte("font"), 0o644)
	if err := os.Symlink(outside, filepath.Join(dir, "link.ttf")); err != nil {
		t.Fatal(err)
	}
	doc := func(font string) string {
		return `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"` + font + `"}]}`
	}
	s, err := Parse([]byte(doc("f.ttf")), 5, testEnv(dir))
	if err != nil {
		t.Fatalf("a font of the sources: %v", err)
	}
	if h := s.FontsHash(dir); h == "" {
		t.Error("a font gives a hash")
	}
	if _, err := Parse([]byte(doc("link.ttf")), 5, testEnv(dir)); err == nil || !strings.Contains(err.Error(), "resolves outside") {
		t.Errorf("a font linked from outside the sources: %v", err)
	}
}

func TestSpec_ScaleIsANumberOrThree(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":1,"keys":[{"t":0,"scale":2},{"t":1,"scale":[1,2,3]}]}]}`
	s, err := Parse([]byte(doc), 5, testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	k := s.Objects[0].Keys
	if k[0].Scale.V != [3]float64{2, 2, 2} || k[1].Scale.V != [3]float64{1, 2, 3} {
		t.Errorf("scales: %+v %+v", k[0].Scale, k[1].Scale)
	}
}

func TestPalette_FromADesign(t *testing.T) {
	p := PaletteFromDesign("#0B0F14", "#fff", "")
	if p.BG != "#0b0f14" || p.FG != "#ffffff" || p.Accent != DefaultPalette.Accent {
		t.Errorf("palette = %+v", p)
	}
	if c, ok := p.Colour("accent"); !ok || c != DefaultPalette.Accent {
		t.Errorf("accent = %q %v", c, ok)
	}
	if c, ok := p.Colour("#AABBCC"); !ok || c != "#aabbcc" {
		t.Errorf("hex = %q %v", c, ok)
	}
	if _, ok := p.Colour("#abc"); ok {
		t.Error("a short hex is not a description colour")
	}
}

func TestSpec_ColoursComeOutResolvedDefaultsIncluded(t *testing.T) {
	doc := `{"version":1,"world":{"color":"#102030"},"camera":{"keys":[{"t":0}]},"lights":[{"type":"point"},{"type":"point","color":"accent"}],` +
		`"materials":{"m":{"color":"accent"},"n":{}}}`
	s, err := Parse([]byte(doc), 5, testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	if s.World.Color != "#102030" || s.Materials["m"].Color != "#ff3b30" || s.Materials["n"].Color != "#e8f1ec" ||
		s.Lights[0].Color != "#ffffff" || s.Lights[1].Color != "#ff3b30" {
		t.Errorf("world %q, m %q, n %q, lights %q %q", s.World.Color, s.Materials["m"].Color, s.Materials["n"].Color, s.Lights[0].Color, s.Lights[1].Color)
	}
	// A description that names no colour of the design is the same whatever the design says; one that does is not.
	plain := `{"version":1,"world":{"color":"#102030"},"camera":{"keys":[{"t":0}]},"materials":{"m":{"color":"#ffffff"}}}`
	json := func(doc string, p Palette) string {
		e := testEnv("")
		e.Palette = p
		s, err := Resolve([]byte(doc), e)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := s.JSON()
		return string(b)
	}
	other := testPalette
	other.Accent = "#00ff00"
	if json(plain, testPalette) != json(plain, other) {
		t.Error("a description with no design colour must resolve the same under another design")
	}
	if json(doc, testPalette) == json(doc, other) {
		t.Error("a description that names accent must resolve differently under another accent")
	}
}

func TestSpec_StepIsAnEaseAndABackdropIsAnObject(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"floor","type":"backdrop","size":20,"curve":2},` +
		`{"id":"a","type":"sphere","size":1,"keys":[{"t":0,"emission":0},{"t":2,"emission":4,"ease":"step"}]}]}`
	if err := parse(t, doc); err != nil {
		t.Errorf("step and backdrop: %v", err)
	}
}

func TestSpec_BackdropRules(t *testing.T) {
	doc := func(objects string) string {
		return `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[` + objects + `]}`
	}
	for name, tc := range map[string]struct{ doc, want string }{
		"no curve":       {doc(`{"id":"f","type":"backdrop","size":20}`), "objects[0].curve: is required for a backdrop"},
		"curve too big":  {doc(`{"id":"f","type":"backdrop","size":20,"curve":11}`), "objects[0].curve: 11 is outside 0.1 to 10"},
		"curve too tiny": {doc(`{"id":"f","type":"backdrop","size":20,"curve":0.05}`), "objects[0].curve"},
		"two":            {doc(`{"id":"f","type":"backdrop","size":20,"curve":2},{"id":"g","type":"backdrop","size":20,"curve":2}`), "objects[1].type: a scene has at most one backdrop"},
		"curve on cube":  {doc(`{"id":"f","type":"cube","size":1,"curve":2}`), "objects[0].curve: curve is only for objects of type backdrop"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := parse(t, tc.doc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
	if err := parse(t, doc(`{"id":"f","type":"backdrop","size":20,"curve":10}`)); err != nil {
		t.Errorf("a curve of half the size is the limit and is valid: %v", err)
	}
}

func TestSpec_BeatsBecomeSecondsWithTheTempo(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":1,"keys":[{"beat":0},{"beat":1},{"beat":2.5},{"t":2.75}]}]}`
	s, err := Parse([]byte(doc), 5, testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0, 0.5, 1.25, 2.75}
	for i, k := range s.Objects[0].Keys {
		if k.T == nil || *k.T != want[i] || k.Beat != nil {
			t.Errorf("key %d: t=%v beat=%v, want t=%v and no beat", i, k.T, k.Beat, want[i])
		}
	}
	e := testEnv("")
	e.BPM = 90 // 90 bpm: a beat lasts 2/3 s
	s, err = Parse([]byte(doc), 5, e)
	if err != nil {
		t.Fatal(err)
	}
	if got := *s.Objects[0].Keys[1].T; got < 0.6666 || got > 0.6667 {
		t.Errorf("beat 1 at 90 bpm = %v s", got)
	}
}

func TestSpec_BeatRules(t *testing.T) {
	noTempo := testEnv("")
	noTempo.BPM = 0
	for name, tc := range map[string]struct {
		doc  string
		env  Env
		want string
	}{
		"without a tempo": {`{"version":1,"camera":{"keys":[{"t":0},{"beat":2}]}}`, noTempo,
			"camera.keys[1].beat: the project has no tempo"},
		"beyond the scene": {`{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"cube","size":1,"keys":[{"beat":1},{"beat":12}]}]}`, testEnv(""),
			"objects[0].keys[1].beat: beat 12 falls at 6 s, outside the scene (0 to 5 s)"},
		"both":     {`{"version":1,"camera":{"keys":[{"t":0,"beat":0}]}}`, testEnv(""), "camera.keys[0]: give t (seconds) or beat, not both"},
		"neither":  {`{"version":1,"camera":{"keys":[{"location":[0,0,0]}]}}`, testEnv(""), "camera.keys[0]: give t (seconds) or beat"},
		"negative": {`{"version":1,"camera":{"keys":[{"beat":-1}]}}`, testEnv(""), "camera.keys[0].beat"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.doc), 5, tc.env)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
	// The beat exactly at the end of the scene is inside it: 10 beats at 120 bpm is 5 s.
	if err := parse(t, `{"version":1,"camera":{"keys":[{"t":0},{"beat":10}]}}`); err != nil {
		t.Errorf("the last beat of the scene: %v", err)
	}
}

func TestSpec_ResolveGivesAKeyToADescriptionThatLoadWouldRefuse(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0},{"beat":12}]}}`
	s, err := Resolve([]byte(doc), testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	if k := s.Camera.Keys[1]; k.T == nil || *k.T != 6 {
		t.Errorf("a beat is converted even when it is beyond the scene: %+v", k)
	}
	e := testEnv("")
	e.BPM = 0
	s, err = Resolve([]byte(doc), e)
	if err != nil || s.Camera.Keys[1].Beat == nil {
		t.Errorf("without a tempo the beat stays as it is: %v %+v", err, s.Camera.Keys[1])
	}
}

func TestSpec_FontRoles(t *testing.T) {
	doc := `{"version":1,"camera":{"keys":[{"t":0}]},"objects":[{"id":"a","type":"text","size":1,"text":"x","font":"display"},` +
		`{"id":"b","type":"text","size":1,"text":"y","font":"body"},{"id":"c","type":"text","size":1,"text":"z","font":"display"}]}`
	s, err := Parse([]byte(doc), 5, testEnv(""))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.FontRoles(), ","); got != "display,body" {
		t.Errorf("roles = %q", got)
	}
	if len(s.Fonts()) != 0 || s.FontsHash("") != "" {
		t.Error("a role is not a file of the sources")
	}
	e := testEnv("")
	e.DesignFonts = map[string]bool{FontBody: true} // the design's display font is a system stack
	if _, err := Parse([]byte(doc), 5, e); err == nil || !strings.Contains(err.Error(), "objects[0].font: the design's display font is not one of the server's font pack") {
		t.Errorf("a display font the pack does not have: %v", err)
	}
}
