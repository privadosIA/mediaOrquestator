// Package providers holds what the program knows about the OpenAI-compatible
// providers it can talk to: their addresses and the models worth starting with.
// It is plain data with no dependencies, so the editions can build their host
// list from it. See specs/model-providers.
package providers

import (
	"net/url"
	"strings"
)

// Preset is a known OpenAI-compatible provider.
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// BaseURL is the OpenAI-compatible address of the provider.
	BaseURL string `json:"base_url"`
	// ListsModels says whether GET <base>/models works with the provider's key.
	ListsModels  bool     `json:"lists_models"`
	TextModels   []string `json:"text_models"`
	VisionModels []string `json:"vision_models"`
	// KeyHint says where the key comes from.
	KeyHint string `json:"key_hint"`
	// Notes carries the caveats of the provider's compatibility layer.
	Notes string `json:"notes"`
}

// ModelsDate is when the suggested model names were checked; they are
// suggestions the person edits.
const ModelsDate = "2026-11-16"

// order is the preference order of the presets (also the order of suggestions).
var presets = []Preset{
	{
		ID: "anthropic", Name: "Claude (Anthropic)", BaseURL: "https://api.anthropic.com/v1/", ListsModels: false,
		TextModels: []string{"claude-sonnet-5-5", "claude-opus-5-5"}, VisionModels: []string{"claude-sonnet-5-5", "claude-opus-5-5"},
		KeyHint: "Claude console → API keys",
		Notes:   "The OpenAI-compatible layer is meant for testing: response_format is not honoured (JSON inside prose is tolerated), thinking is off and the models are not listed, so type their names by hand.",
	},
	{
		ID: "gemini", Name: "Gemini (Google)", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/", ListsModels: true,
		TextModels: []string{"gemini-pro-latest", "gemini-flash-latest"}, VisionModels: []string{"gemini-flash-latest", "gemini-pro-latest"},
		KeyHint: "Google AI Studio → Get API key",
		Notes:   "Google's OpenAI endpoint lives under v1beta; its model list names carry a models/ prefix, which is removed. The -latest names are aliases Google keeps pointing at its current Pro and Flash models; Pro thinks before it answers, so short calls need room in max_tokens.",
	},
	{
		ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1/", ListsModels: true,
		TextModels: []string{"anthropic/claude-sonnet-5.5", "anthropic/claude-opus-5.5"}, VisionModels: []string{"anthropic/claude-sonnet-5.5"},
		KeyHint: "openrouter.ai → Keys",
		Notes:   "Its model list answers without a key, so the test sends a chat to prove the key.",
	},
	{
		ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1/", ListsModels: true,
		TextModels: []string{"gpt-4.1"}, VisionModels: []string{"gpt-4.1"},
		KeyHint: "platform.openai.com → API keys",
	},
}

// Presets returns the known providers in order of preference (a copy).
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// Find returns the preset with that id.
func Find(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// IDs lists the ids of the presets.
func IDs() []string {
	out := make([]string, len(presets))
	for i, p := range presets {
		out[i] = p.ID
	}
	return out
}

// Hosts returns the host of every preset's address, lowercase: the only hosts
// the public edition accepts for a saved provider.
func Hosts() []string {
	out := make([]string, 0, len(presets))
	for _, p := range presets {
		if u, err := url.Parse(p.BaseURL); err == nil {
			out = append(out, strings.ToLower(u.Hostname()))
		}
	}
	return out
}

// Rank is the position of a preset in the order of preference (-1 for none).
func Rank(id string) int {
	for i, p := range presets {
		if p.ID == id {
			return i
		}
	}
	return -1
}
