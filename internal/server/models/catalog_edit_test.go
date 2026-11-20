package models

import (
	"strings"
	"testing"
)

func TestCatalog_FluxKontextIsAnEditingModelAndNotAGenerationOne(t *testing.T) {
	var e *catalogEntry
	for i := range catalog {
		if catalog[i].ID == "image-edit-flux-kontext-q4" {
			e = &catalog[i]
		}
	}
	if e == nil {
		t.Fatal("the editing model is not in the catalog")
	}
	if !e.Edit || e.Kind != "model" || len(e.Styles) != 0 {
		t.Errorf("edit=%v kind=%q styles=%v: it must be an editing model with no look", e.Edit, e.Kind, e.Styles)
	}
	roles := map[string]bool{}
	for _, f := range e.Files {
		roles[f.Role] = true
		if !strings.HasPrefix(f.URL, "https://huggingface.co/") || f.Name == "" {
			t.Errorf("file %+v", f)
		}
	}
	for _, r := range []string{"model", "clip_l", "t5xxl", "vae"} {
		if !roles[r] {
			t.Errorf("missing the %s file", r)
		}
	}
	if e.Timing == nil || !e.Timing.Measured || e.NameEN == "" || e.DescriptionEN == "" || e.VRAMMB == 0 {
		t.Errorf("timing, English texts and memory must be filled: %+v", e)
	}
}

func TestCatalog_QwenImageEditIsASecondEditingModel(t *testing.T) {
	var e *catalogEntry
	for i := range catalog {
		if catalog[i].ID == "image-edit-qwen-image-edit-q4" {
			e = &catalog[i]
		}
	}
	if e == nil {
		t.Fatal("the Qwen editing model is not in the catalog")
	}
	if !e.Edit || e.Kind != "model" || len(e.Styles) != 0 {
		t.Errorf("edit=%v kind=%q styles=%v: it must be an editing model with no look", e.Edit, e.Kind, e.Styles)
	}
	roles := map[string]bool{}
	for _, f := range e.Files {
		roles[f.Role] = true
		if !strings.HasPrefix(f.URL, "https://huggingface.co/") || f.Name == "" {
			t.Errorf("file %+v", f)
		}
	}
	for _, r := range []string{"model", "llm", "llm_vision", "vae"} {
		if !roles[r] {
			t.Errorf("missing the %s file", r)
		}
	}
	if roles["clip_l"] || roles["t5xxl"] {
		t.Error("the Qwen family has no CLIP-L or T5-XXL")
	}
	if e.Timing == nil || !e.Timing.Measured || e.NameEN == "" || e.DescriptionEN == "" || e.VRAMMB == 0 || e.MinRAMGB == 0 {
		t.Errorf("timing, English texts, memory and RAM must be filled: %+v", e)
	}
}
