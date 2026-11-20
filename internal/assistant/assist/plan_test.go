package assist

import (
	"strings"
	"testing"
)

func TestParsePlanReply(t *testing.T) {
	g := ParsePlanReply("<think>hmm</think>\n**TITLE**: Night Drive\nLANG=ES\nINSTRUMENTAL=no\nSECONDS=90 s\nSTYLE=Illustrated\nLEVEL = medium\nFPS=30\nMODE=dark\nCONCEPT=a car at night\n- PART=timeline-strip | a strip of years\nPART=Year-Counter | a counter that rolls through years\nPART=\n")
	if g.Title != "Night Drive" || g.Language != "es" || g.Instrumental || g.Seconds != 90 || g.Style != "illustrated" || g.Level != "medium" || g.FPS != 30 || g.Mode != "dark" || g.Concept != "a car at night" {
		t.Fatalf("%+v", g)
	}
	if len(g.Parts) != 2 || g.Parts[0].Name != "timeline-strip" || g.Parts[1].Name != "year-counter" || g.Parts[1].Request != "a counter that rolls through years" {
		t.Fatalf("%+v", g.Parts)
	}
	if g := ParsePlanReply("I cannot help"); g.Title != "" || len(g.Parts) != 0 {
		t.Fatalf("%+v", g)
	}
}

func TestParsePlanReplyReadsTheStoryline(t *testing.T) {
	g := ParsePlanReply("TITLE=Night\nCONCEPT=a car\nWORLD=a night road, headlights and dust\nDEVICE=One object across the song.\nHOOK=**a pair of headlights cut the dark**\n")
	if g.World != "a night road, headlights and dust" || g.Device != "one object across the song" || g.Hook != "a pair of headlights cut the dark" {
		t.Fatalf("%+v", g)
	}
	for _, d := range PlanDevices {
		if got := ParsePlanReply("DEVICE=" + d).Device; got != d {
			t.Errorf("%q read as %q", d, got)
		}
	}
	if !strings.Contains(planSystem, "WORLD=") || !strings.Contains(planSystem, "DEVICE=") || !strings.Contains(planSystem, "HOOK=") || !strings.Contains(planSystem, "a question the song answers") {
		t.Error("the planner is asked for the three lines and offered the devices")
	}
}
