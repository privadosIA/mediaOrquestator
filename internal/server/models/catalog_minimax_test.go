package models

import (
	"strings"
	"testing"
)

const idMiniMax = "video-minimax-h3-ref2va-q4"

func TestCatalogMiniMaxH3_FourFilesAndTheRAMWarning(t *testing.T) {
	e := catalogEntryByID(t, idMiniMax)
	if e.Kind != "video" || len(e.Files) != 5 {
		t.Fatalf("kind %q files %d", e.Kind, len(e.Files))
	}
	roles := rolesByName(e)
	for _, want := range []string{"model", "text_encoder", "vae", "audio_vae", "lora"} {
		if roles[want] == "" {
			t.Errorf("no %s file", want)
		}
	}
	if e.MinRAMGB != 24 {
		t.Errorf("min_ram_gb = %d, want 24", e.MinRAMGB)
	}
	for _, want := range []string{"--mmap", "30 GB de RAM"} {
		if !strings.Contains(e.Description, want) {
			t.Errorf("the description must state the RAM requirement (%q): %s", want, e.Description)
		}
	}
	for _, f := range e.Files {
		if !strings.HasPrefix(f.URL, "https://") {
			t.Errorf("%s must be an https URL", f.Name)
		}
	}
	if e.Quality != "" {
		t.Errorf("it must stay out of the editor's quality scale, quality = %q", e.Quality)
	}
	if e.RequiresToken != "" {
		t.Errorf("the files are public, requires_token = %q", e.RequiresToken)
	}
}

func TestCatalogMiniMaxH3_InstalledOnlyWithEveryFileInModelsVideo(t *testing.T) {
	s := newAssetServer(t)
	for i, f := range catalogEntryByID(t, idMiniMax).Files {
		if catalogEntryInstalled(t, s, idMiniMax) {
			t.Fatalf("installed after only %d of 5 files", i)
		}
		writeAsset(t, s, "video", f.Name, 1)
	}
	if !catalogEntryInstalled(t, s, idMiniMax) {
		t.Error("with the five files the bundle is installed")
	}
}

func TestCatalogMiniMaxH3FirstFrame_SharesEverythingButTheDiTAndTheLoRA(t *testing.T) {
	ref := catalogEntryByID(t, idMiniMax)
	ff := catalogEntryByID(t, "video-minimax-h3-fl2va-q4")
	if ff.Kind != "video" || len(ff.Files) != 5 || ff.MinRAMGB != 24 || ff.Quality != "" {
		t.Fatalf("entry = %+v", ff)
	}
	rr, rf := rolesByName(ref), rolesByName(ff)
	for _, role := range []string{"text_encoder", "vae", "audio_vae"} {
		if rr[role] != rf[role] {
			t.Errorf("%s must be the same file in both bundles (so it is downloaded once): %q vs %q", role, rr[role], rf[role])
		}
	}
	for _, role := range []string{"model", "lora"} {
		if rr[role] == rf[role] {
			t.Errorf("the %s must differ between Ref2VA and first-frame", role)
		}
	}
	if !strings.Contains(rf["model"], "fl2va") || ff.Timing == nil || ff.Timing.TurboSteps != 8 {
		t.Errorf("model %q timing %+v", rf["model"], ff.Timing)
	}
}

func TestCatalogMiniMaxH3FirstFrame_RecordsHowItFitsAn12GBCard(t *testing.T) {
	ff := catalogEntryByID(t, "video-minimax-h3-fl2va-q4")
	if ff.LowVRAM == nil || ff.LowVRAM.Resolution != "768x416" || ff.LowVRAM.VRAMMB < 11264 || ff.LowVRAM.VRAMMB > 12000 {
		t.Fatalf("low_vram = %+v: measured peak 11.2 GB at 768x416 on a 12 GB card", ff.LowVRAM)
	}
	// The references bundle was measured too (2026-11-07): 2 pictures, a video and an audio peaked at 11.3 GB.
	if ref := catalogEntryByID(t, idMiniMax); ref.LowVRAM == nil || ref.LowVRAM.Resolution != "768x416" || ref.LowVRAM.VRAMMB < 11264 || ref.LowVRAM.VRAMMB > 12000 {
		t.Errorf("the references bundle low_vram = %+v: measured peak 11.3 GB at 768x416 on a 12 GB card", ref.LowVRAM)
	}
}
