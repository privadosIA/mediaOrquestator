package music

import (
	"context"
	"testing"
)

func TestDrafts_RunningCountsTheJobsInFlight(t *testing.T) {
	var d draftState
	d.jobs = map[string]context.CancelFunc{}
	if d.running() != 0 {
		t.Fatal("an empty state has no job in flight")
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		d.jobs[id] = func() {}
	}
	if d.running() < maxDraftJobs {
		t.Fatalf("running() = %d, want %d: draftEngine refuses (429 queue_full) from there on", d.running(), maxDraftJobs)
	}
}
