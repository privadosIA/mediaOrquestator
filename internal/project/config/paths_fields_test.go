package config

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// typeStrings lists the YAML paths (indexes and map keys written as []) of every string field reachable from t.
func typeStrings(t reflect.Type, prefix string, seen map[reflect.Type]bool, out map[string]bool) {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		typeStrings(t.Elem(), prefix+map[bool]string{true: "[]", false: ""}[t.Kind() != reflect.Pointer], seen, out)
	case reflect.Struct:
		if strings.Contains(t.PkgPath(), "yaml") || seen[t] {
			return
		}
		seen[t] = true
		defer delete(seen, t)
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
			if !sf.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(sf.Name)
			}
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			if sf.Type.Kind() == reflect.String || (sf.Type.Kind() == reflect.Slice && sf.Type.Elem().Kind() == reflect.String) {
				out[p] = true
				continue
			}
			typeStrings(sf.Type, p, seen, out)
		}
	}
}

// suspicious names a string field that may hold a place but is not recognised as a path by its name.
var suspicious = regexp.MustCompile(`(?i)folder|location|dest|target|where|uri|url|out$|root|base`)

// TestPathFieldsAreTheOnesReviewed fails when a string field is added whose name makes it a path (it would now be
// checked) or looks like one without matching: whoever adds it decides, and updates this list.
func TestPathFieldsAreTheOnesReviewed(t *testing.T) {
	all := map[string]bool{}
	typeStrings(reflect.TypeOf(Config{}), "", map[reflect.Type]bool{}, all)

	var paths, maybe []string
	for p := range all {
		last := p[strings.LastIndex(p, ".")+1:]
		switch {
		case pathFieldName.MatchString(last) && !notPathFields[p]:
			paths = append(paths, p)
		case suspicious.MatchString(last):
			maybe = append(maybe, p)
		}
	}
	sort.Strings(paths)
	sort.Strings(maybe)
	if len(paths) == 0 {
		t.Fatal("no path fields found: the walk is broken")
	}
	// The reviewed lists. Add a field here only after deciding whether it must stay inside the project.
	wantMaybe := []string(nil)
	if strings.Join(maybe, ",") != strings.Join(wantMaybe, ",") {
		t.Errorf("string fields that look like places but are not checked as paths: %v\n(rename them, add them to notPathFields with a reason, or list them in wantMaybe)", maybe)
	}
	for _, must := range []string{
		"output.file", "output_file", "defaults.model", "defaults.voice", "defaults.animate.diffusion_model",
		"defaults.animate.vae", "defaults.animate.high_noise_diffusion_model", "defaults.music.models_dir",
		"scenes[].image", "scenes[].clip", "scenes[].references.images", "defaults.model_set.diffusion_model",
	} {
		found := false
		for _, p := range paths {
			if strings.ReplaceAll(p, "[]", "") == strings.ReplaceAll(must, "[]", "") || p == must {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is not checked as a path; checked: %v", must, paths)
		}
	}
}
