package models

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// int8Payload is a tiny int8 convrot checkpoint: one 4x64 layer with group size 64 and a scale per row.
func int8Payload() []byte {
	meta := `{"layers":{"b.lin":{"format":"int8_tensorwise","convrot":true,"convrot_groupsize":64}}}`
	hdr, _ := json.Marshal(map[string]any{
		"__metadata__":       map[string]string{"_quantization_metadata": meta},
		"b.lin.weight":       map[string]any{"dtype": "I8", "shape": []int{4, 64}, "data_offsets": []int{0, 256}},
		"b.lin.weight_scale": map[string]any{"dtype": "F32", "shape": []int{4, 1}, "data_offsets": []int{256, 272}},
		"b.lin.comfy_quant":  map[string]any{"dtype": "U8", "shape": []int{2}, "data_offsets": []int{272, 274}},
	})
	for len(hdr)%8 != 0 {
		hdr = append(hdr, ' ')
	}
	out := binary.LittleEndian.AppendUint64(nil, uint64(len(hdr)))
	out = append(out, hdr...)
	out = append(out, make([]byte, 256+16+2)...)
	return out
}

// fakeSD is an sd that "converts" by copying its -m file to its -o file.
func fakeSD(t *testing.T, s *testServer) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sd")
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -m) in=\"$2\";; -o) out=\"$2\";; esac; shift; done\ncp \"$in\" \"$out\"\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s.look = func(string) (string, error) { return p, nil }
}

func convertCatalog(t *testing.T, url string) string {
	t.Helper()
	saved := catalog
	t.Cleanup(func() { catalog = saved })
	catalog = append(append([]catalogEntry{}, catalog...), catalogEntry{
		ID: "test-convert", Kind: "model", Name: "Test convert",
		Files: []catalogFile{{Name: "conv-q8_0.gguf", Role: "model", URL: url, Convert: "int8-convrot:q8_0"}},
	})
	return "test-convert"
}

func TestDownload_ConvertsAnInt8ConvrotFileBeforeInstallingIt(t *testing.T) {
	s := newDLServer(t)
	fakeSD(t, s)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(int8Payload()) }))
	t.Cleanup(src.Close)
	id := convertCatalog(t, src.URL+"/x.safetensors")

	d := startDL(t, s, `{"id":"`+id+`"}`)
	if d.State != dlSucceeded {
		t.Fatalf("state %s: %s", d.State, d.Error)
	}
	dir := s.KindDir(mustKind(t, "model"))
	raw, err := os.ReadFile(filepath.Join(dir, "conv-q8_0.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw[8:200]), `"BF16"`) || strings.Contains(string(raw), "weight_scale") {
		t.Errorf("the installed file is not the converted one: %q", raw[8:120])
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if e.Name() != "conv-q8_0.gguf" {
			t.Errorf("leftover %s after the conversion", e.Name())
		}
	}
}

func TestDownload_ConversionOfAFileThatIsNotInt8InstallsNothing(t *testing.T) {
	s := newDLServer(t)
	fakeSD(t, s)
	src := fileServer(t, "this is not a checkpoint")
	id := convertCatalog(t, src.URL+"/x.safetensors")

	d := startDL(t, s, `{"id":"`+id+`"}`)
	if d.State != dlFailed || !strings.Contains(d.Error, "not an int8 convrot") {
		t.Fatalf("want a failure about the format, got %s %q", d.State, d.Error)
	}
	if _, err := os.Stat(filepath.Join(s.KindDir(mustKind(t, "model")), "conv-q8_0.gguf")); err == nil {
		t.Error("nothing must be installed")
	}
	noPartials(t, s)
}

func TestDownload_ConversionNeedsTheSDBinary(t *testing.T) {
	s := newDLServer(t)
	s.look = func(string) (string, error) { return "", os.ErrNotExist }
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(int8Payload()) }))
	t.Cleanup(src.Close)
	id := convertCatalog(t, src.URL+"/x.safetensors")
	d := startDL(t, s, `{"id":"`+id+`"}`)
	if d.State != dlFailed || !strings.Contains(d.Error, "sd binary") {
		t.Fatalf("got %s %q", d.State, d.Error)
	}
	noPartials(t, s)
}

func mustKind(t *testing.T, name string) AssetKind {
	t.Helper()
	k, ok := FindKind(name)
	if !ok {
		t.Fatalf("no kind %s", name)
	}
	return k
}
