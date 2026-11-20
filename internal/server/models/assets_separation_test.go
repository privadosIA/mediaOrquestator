package models

import "testing"

func TestAssetKinds_TheVoiceSeparatorWeightsAreOneBinFileOfTheirOwnKind(t *testing.T) {
	k, ok := FindKind("separation")
	if !ok {
		t.Fatal("the weights of the voice separator have no asset kind")
	}
	if k.Dir != "models/demucs" || k.Note == "" {
		t.Errorf("kind = %+v, want models/demucs with a note", k)
	}
	if err := ValidateAssetName(k, "ggml-model-htdemucs-4s-f16.bin"); err != nil {
		t.Errorf("the weights name is refused by its own kind: %v", err)
	}
	if err := ValidateAssetName(k, "model.safetensors"); err == nil {
		t.Error("only .bin files are weights of demucs.cpp")
	}
}
