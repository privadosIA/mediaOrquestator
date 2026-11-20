package cache

import (
	"reflect"
	"testing"
)

// Spec cache, "Each input changes only the phases that depend on it": for every field of the hashed parameters,
// changing it changes its own hash, and through Decide invalidates the phases that depend on it and no other. The
// fields are walked by reflection, so a field added to AudioParams or ImageParams without entering its hash fails here.

// filled sets every field of v (a pointer to a struct) to a non-zero value, so every conditional part of a hash is on.
func filled(t *testing.T, v any) {
	t.Helper()
	rv := reflect.ValueOf(v).Elem()
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(rv.Type().Field(i).Name + "-value")
		case reflect.Int, reflect.Int64:
			f.SetInt(int64(i + 2))
		case reflect.Float64:
			f.SetFloat(float64(i) + 1.5)
		case reflect.Bool:
			f.SetBool(true)
		default:
			t.Fatalf("field %s has kind %s: teach the test about it", rv.Type().Field(i).Name, f.Kind())
		}
	}
}

// changed returns a copy of v with field i set to another value.
func changed[T any](v T, i int) T {
	out := v
	f := reflect.ValueOf(&out).Elem().Field(i)
	switch f.Kind() {
	case reflect.String:
		f.SetString(f.String() + "-other")
	case reflect.Int, reflect.Int64:
		f.SetInt(f.Int() + 1)
	case reflect.Float64:
		f.SetFloat(f.Float() + 1)
	case reflect.Bool:
		f.SetBool(!f.Bool())
	}
	return out
}

func TestEveryAudioInputChangesTheAudioHashAndOnlyTheAudioPhases(t *testing.T) {
	var base AudioParams
	filled(t, &base)
	typ := reflect.TypeOf(base)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			next := changed(base, i)
			if AudioHash(next) == AudioHash(base) {
				t.Fatalf("changing %s does not change the audio hash: a narration made with another %s would be reused", name, name)
			}
			m := &Manifest{Scenes: map[string]SceneEntry{"s": {AudioHash: AudioHash(base), ImageHash: "img", ClipHash: "clip"}}}
			d := Decide(m, "s", AudioHash(next), "img", "clip", false)
			if d.Phases[PhaseTTS] || !d.Phases[PhaseRender] || d.Phases[PhaseClip] {
				t.Errorf("%s changed: tts=%v render=%v clip=%v, want tts miss, render hit, clip miss", name, d.Phases[PhaseTTS], d.Phases[PhaseRender], d.Phases[PhaseClip])
			}
		})
	}
}

func TestEveryImageInputChangesTheImageHashAndOnlyTheImagePhases(t *testing.T) {
	var base ImageParams
	filled(t, &base)
	typ := reflect.TypeOf(base)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			next := changed(base, i)
			if ImageHash(next) == ImageHash(base) {
				t.Fatalf("changing %s does not change the image hash: a picture made with another %s would be reused", name, name)
			}
			m := &Manifest{Scenes: map[string]SceneEntry{"s": {AudioHash: "aud", ImageHash: ImageHash(base), ClipHash: "clip"}}}
			d := Decide(m, "s", "aud", ImageHash(next), "clip", false)
			if !d.Phases[PhaseTTS] || d.Phases[PhaseRender] || d.Phases[PhaseClip] {
				t.Errorf("%s changed: tts=%v render=%v clip=%v, want tts hit, render miss, clip miss", name, d.Phases[PhaseTTS], d.Phases[PhaseRender], d.Phases[PhaseClip])
			}
		})
	}
}

func TestTheSameInputsGiveTheSameHash(t *testing.T) {
	var a AudioParams
	var i ImageParams
	filled(t, &a)
	filled(t, &i)
	ah, ih := AudioHash(a), ImageHash(i)
	if ah != AudioHash(a) || ih != ImageHash(i) {
		t.Error("a hash is not deterministic")
	}
	b := a
	if AudioHash(a) != AudioHash(b) {
		t.Error("equal parameters hash differently")
	}
}
