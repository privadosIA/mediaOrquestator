package models

import "testing"

func TestCatalogTiming_ImageModelsAndVideoBundlesCarryIt(t *testing.T) {
	for _, e := range catalog {
		if e.Kind != "model" && e.Kind != "video" {
			if e.Timing != nil {
				t.Errorf("%s (%s) should not carry a timing", e.ID, e.Kind)
			}
			continue
		}
		if e.Timing == nil || e.Timing.Seconds <= 0 || e.Timing.Resolution == "" || e.Timing.Steps <= 0 {
			t.Errorf("%s has no usable timing: %+v", e.ID, e.Timing)
			continue
		}
		if e.Kind == "video" && e.Timing.Frames <= 0 {
			t.Errorf("%s is a clip: its timing needs the frames", e.ID)
		}
	}
	h3 := catalogEntryByID(t, "video-minimax-h3-ref2va-q4")
	if h3.Timing.Seconds != 258 || h3.Timing.TurboSeconds != 96 || h3.Timing.TurboSteps != 4 || !h3.Timing.Measured {
		t.Errorf("MiniMax H3 timing = %+v", h3.Timing)
	}
}
