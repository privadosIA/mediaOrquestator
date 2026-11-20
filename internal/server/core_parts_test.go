package server

import (
	"strings"
	"testing"
)

// The server runs projects too: it hands the catalog of the music motion parts to the runs and to the validation of every
// configuration it saves (it no longer happens when the engine is linked in).
func TestTheServerHandsTheCatalogOfPartsToTheWorkspace(t *testing.T) {
	s := newTestServer(t)
	if (workspaceHost{s}).Motion() == nil {
		t.Fatal("the runs are given no builder for the pages of scenes made from a part")
	}
	parts := s.workspace.Parts()
	if parts == nil {
		t.Fatal("the configurations are validated with no catalog of parts")
	}
	if err := parts.Check("no-such-part", nil); err == nil || !strings.Contains(err.Error(), "not in the catalog") {
		t.Errorf("the catalog does not know the parts: %v", err)
	}
}
