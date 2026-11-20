package music

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mediaOrquestator/internal/assistant/assist"
	"mediaOrquestator/internal/music/musicmotion"
)

func datasetSeam(s *testServer, entries []assist.DatasetEntry, err error) *assist.DatasetInput {
	var seen assist.DatasetInput
	s.Assistant.AssistDatasetRun = func(ctx context.Context, in assist.DatasetInput, model string) ([]assist.DatasetEntry, error) {
		seen = in
		return entries, err
	}
	return &seen
}

func postDataset(t *testing.T, s *testServer, body map[string]any) (int, DatasetAnswer, string) {
	t.Helper()
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/assist/dataset", body)
	var a DatasetAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	return rec.Code, a, rec.Body.String()
}

func TestAssistDataset_VideoScenesGetTheSharedLines(t *testing.T) {
	s := mmServer(t)
	datasetSeam(s, []assist.DatasetEntry{
		{ID: "S01", Title: "The hive", Body: "Dialogue: Al amanecer\nMotion: slow push-in\nNegative: whatever\na wooden beehive in a meadow."},
		{ID: "s01", Title: "Honey", Body: "**Picture:** a jar of honey"},
		{ID: "s03", Title: "No picture", Body: "Motion: pan"},
	}, nil)
	code, a, body := postDataset(t, s, map[string]any{"mode": "video", "text": "bees", "negative": "text, watermark", "look": "documentary", "images": map[string]int{"variants": 2}})
	if code != http.StatusOK || a.Mode != "video" || len(a.Entries) != 2 {
		t.Fatalf("%d %s", code, body)
	}
	e := a.Entries[0]
	for _, want := range []string{"Dialogue: \"Al amanecer\"", "Motion: slow push-in", "a wooden beehive in a meadow., documentary", "Negative: text, watermark", "Variants: 2"} {
		if !strings.Contains(e[3], want) && !strings.Contains(strings.ReplaceAll(e[3], ".,", ","), strings.ReplaceAll(want, ".,", ",")) {
			t.Errorf("entry lacks %q:\n%s", want, e[3])
		}
	}
	if strings.Contains(a.Entries[1][3], "Picture") || !strings.HasPrefix(a.Entries[1][3], "a jar of honey") {
		t.Errorf("the label before the picture is not part of the prompt: %q", a.Entries[1][3])
	}
	if strings.Contains(e[3], "whatever") || e[0] != "s01" || a.Entries[1][0] == "s01" {
		t.Errorf("the request's negative replaces the model's and ids are unique: %v", a.Entries)
	}
	if len(a.Warnings) != 2 {
		t.Errorf("a repeated id and a scene with no picture each warn: %v", a.Warnings)
	}
}

func TestAssistDataset_MotionPartsAreCheckedAgainstTheCatalog(t *testing.T) {
	s := mmServer(t)
	existing := musicmotion.NewCatalog(nil).List()[0].Name
	datasetSeam(s, []assist.DatasetEntry{
		{ID: "g1", Title: "A part", Body: "Part: " + existing + "\nDuration: 4"},
		{ID: "g2", Title: "Unknown", Body: "Part: no-such-part\nDuration: 4"},
		{ID: "g3", Title: "Free", Body: "Graphic: a red bar chart growing\nDuration: 5"},
	}, nil)
	code, a, body := postDataset(t, s, map[string]any{"mode": "motion", "text": "three graphics"})
	if code != http.StatusOK || len(a.Entries) != 2 || a.Entries[0][0] != "g1" || a.Entries[1][0] != "g3" {
		t.Fatalf("%d %s", code, body)
	}
	if len(a.Warnings) != 1 || !strings.Contains(a.Warnings[0], "g2") {
		t.Fatalf("%v", a.Warnings)
	}
}

func TestAssistDataset_MusicEntriesAreTracks(t *testing.T) {
	s := mmServer(t)
	datasetSeam(s, []assist.DatasetEntry{{ID: "t1", Title: "Night", Body: "synthwave, driving\nInstrumental: yes"}, {ID: "t2", Title: "Sun", Body: "folk ballad"}}, nil)
	planSeams(s, assist.PlanGuess{}, nil, nil)
	code, a, body := postDataset(t, s, map[string]any{"mode": "music", "text": "two songs", "generator": "heartmula"})
	if code != http.StatusOK || len(a.Entries) != 2 {
		t.Fatalf("%d %s", code, body)
	}
	if !strings.Contains(a.Entries[0][3], "Lyrics: [Instrumental]") || !strings.Contains(a.Entries[1][3], "Lyrics:\n[verse]") || !strings.Contains(a.Entries[1][3], "Tags: synthwave,neon") {
		t.Fatalf("%v", a.Entries)
	}
}

func TestAssistDataset_RefusalsAndBadRequests(t *testing.T) {
	s := mmServer(t)
	datasetSeam(s, nil, assist.ErrRefused)
	if code, _, body := postDataset(t, s, map[string]any{"mode": "video", "text": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", code, body)
	}
	datasetSeam(s, []assist.DatasetEntry{{ID: "a", Body: "Motion: pan"}}, nil)
	if code, _, body := postDataset(t, s, map[string]any{"mode": "video", "text": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("nothing usable is 422: %d %s", code, body)
	}
	six := []map[string]string{{"prompt": "1"}, {"prompt": "2"}, {"prompt": "3"}, {"prompt": "4"}, {"prompt": "5"}, {"prompt": "6"}}
	for _, body := range []map[string]any{
		{"mode": "audio", "text": "x"}, {"text": "x"}, {"mode": "video", "text": " "}, {"mode": "video", "text": "x", "language": "español"},
		{"mode": "video", "text": "x", "provider": "nope"}, {"mode": "video", "text": "x", "images": map[string]int{"variants": 0}},
		{"mode": "video", "text": "x", "examples": six},
	} {
		if code, _, b := postDataset(t, s, body); code != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", body, code, b)
		}
	}
}

func TestAssistDataset_TheCatalogSaysItExists(t *testing.T) {
	has := map[string]bool{}
	for _, f := range musicmotion.Features {
		has[f] = true
	}
	if !has["dataset"] || !has["examples"] {
		t.Fatal(musicmotion.Features)
	}
}

func TestAssistPlan_SecondsGivenAreUsedAndNotWarnedAbout(t *testing.T) {
	s := mmServer(t)
	var seen assist.PlanInput
	planSeams(s, assist.PlanGuess{Title: "x", Style: "illustrated", Level: "medium", FPS: 30, Mode: "dark", Concept: "c", World: "a city", Device: "a number lands", Hook: "a door opens"}, nil, nil)
	inner := s.Assistant.AssistPlanRun
	s.Assistant.AssistPlanRun = func(ctx context.Context, in assist.PlanInput, model string) (assist.PlanGuess, error) {
		seen = in
		return inner(ctx, in, model)
	}
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/assist/plan", map[string]any{"text": "algo", "seconds": 90})
	var a PlanAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if rec.Code != http.StatusOK || a.Song.Seconds != 90 || seen.Seconds != 90 || len(a.Warnings) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for _, v := range []int{-1, 601} {
		if rec := doJSON(t, s.Handler(), http.MethodPost, "/api/assist/plan", map[string]any{"text": "algo", "seconds": v}); rec.Code != http.StatusBadRequest {
			t.Fatalf("%d: %d", v, rec.Code)
		}
	}
	rec = doJSON(t, s.Handler(), http.MethodPost, "/api/assist/plan", map[string]any{"text": "algo", "examples": []map[string]string{{"prompt": ""}}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
