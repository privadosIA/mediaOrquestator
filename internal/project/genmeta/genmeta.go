// Package genmeta reads the generation settings a picture or clip carries
// (prompt, negative prompt, seed, steps, cfg, model) from the formats that
// embed them: PNG text chunks (stable-diffusion.cpp and A1111 "parameters",
// ComfyUI "prompt"), video container tags, the comment of a GIF and the EXIF
// and XMP of a WebP. The input is untrusted: every read is bounded, numbers
// are range-checked, and a file with nothing readable yields an empty result,
// never an error.
package genmeta

import (
	"strings"
	"unicode/utf8"
)

// Bounds of a read (see the generation-metadata spec).
const (
	// MaxInspect is how much of a file is looked at.
	MaxInspect = 8 << 20
	// MaxValue is the longest text value kept.
	MaxValue = 64 << 10
	// MaxDecompressed is the most a compressed chunk may expand to.
	MaxDecompressed = 1 << 20
	// MaxGraphNodes is the largest ComfyUI graph read.
	MaxGraphNodes = 2000
	// MaxPromptRunes clips the prompt and the negative prompt.
	MaxPromptRunes = 20000
)

// Metadata is what was found; every field is optional.
type Metadata struct {
	Prompt         string   `json:"prompt,omitempty"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
	Seed           *int64   `json:"seed,omitempty"`
	Steps          *int     `json:"steps,omitempty"`
	CfgScale       *float64 `json:"cfg_scale,omitempty"`
	Sampler        string   `json:"sampler,omitempty"`
	Model          string   `json:"model,omitempty"`
	Width          *int     `json:"width,omitempty"`
	Height         *int     `json:"height,omitempty"`
	Strength       *float64 `json:"strength,omitempty"`
}

// Empty reports whether nothing useful was found.
func (m Metadata) Empty() bool {
	return m.Prompt == "" && m.NegativePrompt == "" && m.Seed == nil && m.Steps == nil &&
		m.CfgScale == nil && m.Sampler == "" && m.Model == "" && m.Width == nil && m.Height == nil && m.Strength == nil
}

// FromBytes reads the metadata of a PNG, GIF or WebP by its signature. Any
// other content (a JPEG, a video, garbage) yields none: videos go through
// FromVideoTags.
func FromBytes(data []byte) Metadata {
	if len(data) > MaxInspect {
		data = data[:MaxInspect]
	}
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return fromPNG(data)
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return fromGIF(data)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return fromWebP(data)
	}
	return Metadata{}
}

// FromVideoTags reads the container tags a video carries (the comment and
// description that stable-diffusion.cpp and ffmpeg keep); keys are matched
// case-insensitively.
func FromVideoTags(tags map[string]string) Metadata {
	for _, want := range []string{"comment", "description", "parameters"} {
		for k, v := range tags {
			if strings.EqualFold(k, want) {
				if m := FromText(v); !m.Empty() {
					return m
				}
			}
		}
	}
	return Metadata{}
}

// FromText reads a generation text in whichever shape it has: ComfyUI's JSON
// graph, or the A1111 / stable-diffusion.cpp "parameters" text.
func FromText(text string) Metadata {
	text = strings.TrimSpace(text)
	if len(text) > MaxValue*4 {
		return Metadata{}
	}
	if strings.HasPrefix(text, "{") {
		return FromComfyPrompt([]byte(text))
	}
	return fromParameters(text)
}

// clipText cuts s to at most n runes, on a rune boundary, dropping invalid UTF-8.
func clipText(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func sanitize(m Metadata) Metadata {
	m.Prompt = clipText(strings.TrimSpace(m.Prompt), MaxPromptRunes)
	m.NegativePrompt = clipText(strings.TrimSpace(m.NegativePrompt), MaxPromptRunes)
	m.Sampler = clipText(strings.TrimSpace(m.Sampler), 200)
	m.Model = clipText(strings.TrimSpace(m.Model), 500)
	if m.Steps != nil && (*m.Steps < 1 || *m.Steps > 1000) {
		m.Steps = nil
	}
	if m.CfgScale != nil && (*m.CfgScale < 0 || *m.CfgScale > 100) {
		m.CfgScale = nil
	}
	// A size is both sides or nothing.
	if m.Width != nil && m.Height != nil &&
		(*m.Width < 1 || *m.Width > 16384 || *m.Height < 1 || *m.Height > 16384) {
		m.Width, m.Height = nil, nil
	} else if (m.Width == nil) != (m.Height == nil) {
		m.Width, m.Height = nil, nil
	}
	if m.Strength != nil && (*m.Strength < 0 || *m.Strength > 1) {
		m.Strength = nil
	}
	return m
}
