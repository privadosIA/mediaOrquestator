package workspace

import "testing"

func TestSplitRetouch_RetouchFilesLeaveTheImageModels(t *testing.T) {
	models, retouch := splitRetouch(catalogForTest(t), []AssetEntry{
		{Name: "sd_xl_base_1.0.safetensors"},
		{Name: "Qwen-Image-Edit-2509-Q4_K_M.gguf"},
		{Name: "flux1-kontext-dev-Q4_K_M.gguf"},
		{Name: "qwen_image_vae.safetensors"},
	})
	if len(models) != 1 || models[0].Name != "sd_xl_base_1.0.safetensors" {
		t.Errorf("models = %+v", models)
	}
	if len(retouch) != 3 {
		t.Errorf("retouch = %+v", retouch)
	}
}

func TestSplitRetouch_NothingToSplitGivesEmptyListsNotNull(t *testing.T) {
	models, retouch := splitRetouch(catalogForTest(t), nil)
	if models == nil || retouch == nil {
		t.Errorf("models = %v, retouch = %v: they must be empty lists so the JSON has [] and not null", models, retouch)
	}
}
