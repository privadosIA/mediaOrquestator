package music

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSVG_TheCatalogSaysTheServerCanWriteOne(t *testing.T) {
	s := mmServer(t)
	var c PieceCatalog
	rec := doJSON(t, s.Handler(), http.MethodGet, "/api/music-motion/catalog", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	has := false
	for _, f := range c.Features {
		has = has || f == "svg"
	}
	if rec.Code != http.StatusOK || !has {
		t.Fatalf("%d features %v", rec.Code, c.Features)
	}
}
