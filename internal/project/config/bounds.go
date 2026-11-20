package config

import (
	"fmt"
	"reflect"
	"strings"
)

// Upper bounds of the numbers that drive a job. A scenes.yaml can reach the server from a request, and a value
// such as steps: 100000 or a 16384x16384 picture would keep a GPU or the disk busy for hours, so validation
// refuses what no real preset uses. Each limit is generous: the largest value the documentation, the examples and
// the catalog presets use is far below it.
const (
	MaxSteps          = 200  // sampling steps (steps, high_noise_steps, inference_steps, dr2_steps)
	MaxFPS            = 240  // frames per second
	MaxFrames         = 1000 // frames of one animated clip
	MaxVariants       = 64   // variants of one scene or track
	MaxSilenceSeconds = 3600 // silence_seconds of a silent scene
	MaxDurationSecs   = 3600 // duration_seconds of a track
	MaxMP3Bitrate     = 640  // kbit/s
	MaxSide           = 4096 // one side of a resolution, in pixels
	MaxPixels         = 4096 * 2304
)

// numberLimits maps the YAML name of a numeric field to its upper bound.
var numberLimits = map[string]float64{
	"steps": MaxSteps, "high_noise_steps": MaxSteps, "inference_steps": MaxSteps, "dr2_steps": MaxSteps,
	"fps": MaxFPS, "frames": MaxFrames, "variants": MaxVariants,
	"silence_seconds": MaxSilenceSeconds, "duration_seconds": MaxDurationSecs, "mp3_bitrate": MaxMP3Bitrate,
}

// checkBounds returns one error per number above its limit and per resolution above MaxSide or MaxPixels. The
// lower bounds and the shape rules stay with the checks of each engine.
func checkBounds(c *Config) []error {
	var errs []error
	walkLeaves(reflect.ValueOf(c).Elem(), "", func(field string, v reflect.Value) {
		name := field[strings.LastIndexAny(field, ".")+1:]
		switch v.Kind() {
		case reflect.Int, reflect.Int64:
			if max, ok := numberLimits[name]; ok && float64(v.Int()) > max {
				errs = append(errs, fmt.Errorf("config: %s is %d, above the limit of %v", field, v.Int(), max))
			}
		case reflect.Float64:
			if max, ok := numberLimits[name]; ok && v.Float() > max {
				errs = append(errs, fmt.Errorf("config: %s is %v, above the limit of %v", field, v.Float(), max))
			}
		case reflect.String:
			if name != "resolution" || v.String() == "" {
				return
			}
			w, h, err := parseWxH(v.String())
			if err != nil {
				return // the shape of the value is reported by the check of its engine
			}
			if w > MaxSide || h > MaxSide || w*h > MaxPixels {
				errs = append(errs, fmt.Errorf("config: %s is %q, above the limit of %d pixels a side and %d pixels in all", field, v.String(), MaxSide, MaxPixels))
			}
		}
	})
	return errs
}

// walkLeaves calls fn for every number and string reachable from v with its YAML path; pointers are followed.
func walkLeaves(v reflect.Value, path string, fn func(field string, v reflect.Value)) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkLeaves(v.Elem(), path, fn)
		}
	case reflect.Struct:
		t := v.Type()
		if strings.Contains(t.PkgPath(), "yaml") {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
			if !sf.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(sf.Name)
			}
			sub := name
			if path != "" {
				sub = path + "." + name
			}
			walkLeaves(v.Field(i), sub, fn)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkLeaves(v.Index(i), fmt.Sprintf("%s[%d]", path, i), fn)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkLeaves(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k), fn)
		}
	case reflect.Int, reflect.Int64, reflect.Float64, reflect.String:
		fn(path, v)
	}
}
