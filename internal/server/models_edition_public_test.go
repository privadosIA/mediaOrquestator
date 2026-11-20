//go:build !private

package server

import (
	"net/http"
	"strings"
	"testing"
)

// The public edition has no Civitai: no catalog entry that needs its key, no key,
// no lookup and no download from it.
func TestPublicEditionHasNoCivitai(t *testing.T) {
	for _, e := range catalog {
		if e.RequiresToken == "civitai" {
			t.Errorf("%s needs a Civitai key", e.ID)
		}
	}
	s := newDLServer(t)
	h := s.Handler()
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/tokens/civitai", `{"token":"abcdefghijklmnop"}`},
		{http.MethodDelete, "/api/tokens/civitai", ""},
		{http.MethodGet, "/api/civitai/resolve?ref=https://civitai.com/models/1", ""},
	} {
		if rec := call(h, c.method, c.path, c.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	rec := call(h, http.MethodPost, "/api/downloads", `{"kind":"model","url":"https://civitai.com/api/download/models/1","name":"x.safetensors"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Civitai") {
		t.Errorf("a Civitai download must be refused: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, http.MethodPut, "/api/tokens/huggingface", `{"token":"hf_abcdefghijklmnop"}`); rec.Code == http.StatusNotFound {
		t.Errorf("the Hugging Face key must still work: %s", rec.Body)
	}
}

func TestPublicEditionRefusesProvidersWithAFreeAddress(t *testing.T) {
	isolateSettings(t) // the provider it saves must not be there for the next run
	s, _ := newProjectsServer(t)
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/providers", map[string]any{
		"id": "mine", "name": "Mine", "type": "openai", "base_url": "https://my-server.example/v1", "text_model": "m",
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not allowed") {
		t.Fatalf("a free address must be refused: %d %s", rec.Code, rec.Body)
	}
	rec = doJSON(t, s.Handler(), http.MethodPost, "/api/providers", map[string]any{
		"id": "oa", "name": "OpenAI", "type": "openai", "base_url": "https://api.openai.com/v1", "text_model": "gpt-4o",
	})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("a listed host is accepted: %d %s", rec.Code, rec.Body)
	}
}

func TestPublicEditionReportsItself(t *testing.T) {
	s, _ := newProjectsServer(t)
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/environment", nil)
	if !strings.Contains(rec.Body.String(), `"edition":"public"`) {
		t.Errorf("environment: %s", rec.Body)
	}
}
