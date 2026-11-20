package server

// music_openapi_test.go: the description of the music mode in openapi.yaml.

import (
	"encoding/json"
	"strings"
	"testing"
)

// subtree returns the JSON of a node of the OpenAPI document, addressed by
// keys, so a test can look for a value inside just that part of it.
func subtree(t *testing.T, keys ...string) string {
	t.Helper()
	raw, err := openapiDocument()
	if err != nil {
		t.Fatal(err)
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("%v: not an object at %q", keys, k)
		}
		if node, ok = m[k]; !ok {
			t.Fatalf("%v: no %q in the API description", keys, k)
		}
	}
	b, _ := json.Marshal(node)
	return string(b)
}

func TestOpenAPI_DescribesMusicMode(t *testing.T) {
	for _, c := range []struct {
		what string
		keys []string
		want string
	}{
		{"project summary mode", []string{"components", "schemas", "ProjectSummary", "properties", "mode"}, "music"},
		{"project summary track count", []string{"components", "schemas", "ProjectSummary", "properties"}, "track_count"},
		{"run launch mode", []string{"paths", "/api/projects/{id}/runs", "post", "requestBody"}, "music"},
		{"asset delete kind", []string{"paths", "/api/assets/{kind}/{name}", "delete", "parameters"}, "music"},
		{"download kind", []string{"paths", "/api/downloads", "post", "requestBody"}, "music"},
		{"artifact mp3", []string{"paths", "/api/projects/{id}/artifacts/{path}", "get", "responses"}, "audio/mpeg"},
		{"artifact partial content", []string{"paths", "/api/projects/{id}/artifacts/{path}", "get", "responses"}, `"206"`},
		{"run events compose", []string{"paths", "/api/runs/{id}/events", "get", "description"}, "compose"},
		{"run events synth", []string{"paths", "/api/runs/{id}/events", "get", "description"}, "synth"},
		{"environment ace-lm", []string{"paths", "/api/environment", "get", "description"}, "ace-lm"},
		{"environment ace-synth", []string{"paths", "/api/environment", "get", "description"}, "ace-synth"},
		{"catalog music bundles", []string{"paths", "/api/assets/catalog", "get", "description"}, "music"},
		{"config tracks", []string{"paths", "/api/projects/{id}/config", "get", "description"}, "tracks"},
		{"cache report phases", []string{"paths", "/api/projects/{id}/cache", "get", "description"}, "compose"},
		{"asset delete adapter kind", []string{"paths", "/api/assets/{kind}/{name}", "delete", "parameters"}, "adapter"},
		{"download adapter kind", []string{"paths", "/api/downloads", "post", "requestBody"}, "adapter"},
		{"environment adapters", []string{"paths", "/api/environment", "get", "description"}, "music_adapters"},
		{"catalog folder names", []string{"paths", "/api/assets/catalog", "get", "description"}, "single level"},
		{"assist music task", []string{"paths", "/api/assist", "post", "requestBody"}, "music"},
		{"assist instrumental", []string{"paths", "/api/assist", "post", "requestBody"}, "instrumental"},
		{"assist caption", []string{"paths", "/api/assist", "post", "responses"}, "caption"},
		{"assist lyrics", []string{"paths", "/api/assist", "post", "responses"}, "lyrics"},
	} {
		if got := subtree(t, c.keys...); !strings.Contains(got, c.want) {
			t.Errorf("%s: the API description must mention %q: %s", c.what, c.want, got)
		}
	}
}
