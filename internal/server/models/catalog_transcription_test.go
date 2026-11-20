package models

import "testing"

func TestCatalogTranscription_WhisperModelIsOneBinFileOfAKnownKind(t *testing.T) {
	var found []catalogEntry
	for _, e := range catalog {
		if e.Kind == "transcription" {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected the Whisper model, got %d entries", len(found))
	}
	e := found[0]
	k, ok := FindKind(e.Kind)
	if !ok {
		t.Fatalf("kind %q is not an asset kind", e.Kind)
	}
	if len(e.Files) != 1 || e.Files[0].Role != "asr" {
		t.Fatalf("files: %+v", e.Files)
	}
	if err := ValidateAssetName(k, e.Files[0].Name); err != nil {
		t.Fatalf("the file name is refused by its own kind: %v", err)
	}
	if e.SizeBytes != 574041195 {
		t.Errorf("size %d is not the one of ggml-large-v3-turbo-q5_0.bin", e.SizeBytes)
	}
}
