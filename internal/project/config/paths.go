package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

// Paths in a scenes.yaml stay inside the project. The server loads this file from a request, and the
// pipeline hands its paths to ffmpeg, piper and sd (whose working directory is the project), so a path
// that is absolute or climbs out with ".." would read or write anywhere the server's user can.
//
// Which fields are paths is decided by the field's YAML name (pathFieldName): every string field, in
// every nested struct, slice or map, whose name says it holds a file or a folder is checked. Names
// of models, voices or adapters pass too, because the rule only refuses a leading "/" or a ".."
// segment. TestEveryStringFieldIsClassified makes a new field choose a side.

// pathFieldName matches the YAML names of the fields that hold a file or a folder.
var pathFieldName = regexp.MustCompile(`(?i)file|dir|path|image|clip|video|audio|model|voice|llm|vae|lora|adapter|checkpoint|t5xxl|reference|source|picture|mask`)

// notPathFields lists the string fields whose name matches pathFieldName but that never hold a path
// (so they are not checked), by their YAML path with indexes removed.
var notPathFields = map[string]bool{}

// sceneIDPattern is the shape of a scene id and of the project name: they become file names
// (tmp/scene_<id>.wav, <id>.mp4, out/<project>).
var sceneIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// relativeInside returns "" when p is a relative path with no ".." segment, and the reason otherwise.
func relativeInside(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || (len(p) > 1 && p[1] == ':') {
		return "it is an absolute path"
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return `it has a ".." segment`
		}
	}
	return ""
}

// checkPaths returns one error per path field of c that leaves the project, plus the checks of the
// output file, the project name and the scene ids.
func checkPaths(c *Config) []error {
	var errs []error
	walkStrings(reflect.ValueOf(c).Elem(), "", func(field, value string, isPath bool) {
		if !isPath {
			return
		}
		if why := relativeInside(value); why != "" {
			errs = append(errs, fmt.Errorf("config: %s is %q: %s; paths are relative to the project folder (link the file into the project, e.g. under models/ or sources/)", field, value, why))
		}
	})
	for _, f := range []struct{ name, v string }{{"output.file", c.Output.File}, {"output_file", c.OutputFile}} {
		if f.v == "" || relativeInside(f.v) != "" {
			continue
		}
		if dir := filepath.Dir(filepath.Clean(f.v)); dir != "." && dir != "out" && !strings.HasPrefix(filepath.ToSlash(dir), "out/") {
			errs = append(errs, fmt.Errorf("config: %s is %q: the output goes under out/ (for example out/final.mp4) or is a plain file name", f.name, f.v))
		}
	}
	if c.Project != "" && !sceneIDPattern.MatchString(c.Project) {
		errs = append(errs, fmt.Errorf("config: project %q is not valid: use letters, digits, '.', '_' and '-' (at most 64, not starting with a symbol)", c.Project))
	}
	for i, s := range c.Scenes {
		if !sceneIDPattern.MatchString(s.ID) {
			errs = append(errs, fmt.Errorf("config: scene %d has id %q, which is not valid: use letters, digits, '.', '_' and '-' (at most 64, not starting with a symbol)", i, s.ID))
		}
	}
	return errs
}

// walkStrings calls fn for every string held by v (a struct, pointer, slice or map reached from it),
// with its YAML path (scenes[2].image) and whether its field name is a path name.
func walkStrings(v reflect.Value, path string, fn func(field, value string, isPath bool)) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkStrings(v.Elem(), path, fn)
		}
	case reflect.Struct:
		t := v.Type()
		if strings.Contains(t.PkgPath(), "yaml") {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if !sf.IsExported() {
				continue
			}
			name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(sf.Name)
			}
			sub := name
			if path != "" {
				sub = path + "." + name
			}
			f := v.Field(i)
			if f.Kind() == reflect.String {
				fn(sub, f.String(), pathFieldName.MatchString(name) && !notPathFields[stripIndexes(sub)])
				continue
			}
			walkStrings(f, sub, fn)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			if e.Kind() == reflect.String {
				// A list of strings is a path list when its field name says so; the caller passed that name in path.
				last := path
				if j := strings.LastIndex(last, "."); j >= 0 {
					last = last[j+1:]
				}
				fn(fmt.Sprintf("%s[%d]", path, i), e.String(), pathFieldName.MatchString(stripIndexes(last)) && !notPathFields[stripIndexes(path)])
				continue
			}
			walkStrings(e, fmt.Sprintf("%s[%d]", path, i), fn)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkStrings(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k), fn)
		}
	}
}

var indexes = regexp.MustCompile(`\[[^\]]*\]`)

func stripIndexes(s string) string { return indexes.ReplaceAllString(s, "") }
