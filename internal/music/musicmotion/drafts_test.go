package musicmotion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// scriptedWriter answers with the next of its answers and records what it was asked.
type scriptedWriter struct {
	mu      sync.Mutex
	answers []string
	err     error
	asked   []string
}

func (w *scriptedWriter) Write(ctx context.Context, system, user string, max int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.asked = append(w.asked, user)
	if w.err != nil {
		return "", w.err
	}
	i := len(w.asked) - 1
	if i >= len(w.answers) {
		i = len(w.answers) - 1
	}
	return w.answers[i], nil
}

func answerOf(manifest, fragment string) string {
	return "```json\n" + manifest + "\n```\n```html\n" + fragment + "\n```\n"
}

func draftEngine(t *testing.T, w Writer, describe func(context.Context, []byte) (string, error)) *DraftEngine {
	t.Helper()
	root := t.TempDir()
	return &DraftEngine{Store: NewDraftStore(filepath.Join(root, "drafts")), Cat: Catalog{Builtin: BuiltinParts(), UserDir: filepath.Join(root, "parts")},
		Browser: browserOrSkip(t), Writer: w, Describe: describe}
}

func TestADraftIsReadyAtTheFirstAttemptWithAPreview(t *testing.T) {
	w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment)}}
	var described int
	e := draftEngine(t, w, func(ctx context.Context, pic []byte) (string, error) { described++; return "black and gold", nil })
	d, err := e.Store.Create("a gold dot", "upload", []byte("jpg"))
	if err != nil {
		t.Fatal(err)
	}
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Attempts != 1 || got.Name != "probe" || got.Description != "black and gold" || len(got.Preview) != DraftPreviewFrames || described != 1 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Manifest, "a test part") || !strings.Contains(got.Fragment, "__partSeek") {
		t.Fatal("the files are kept")
	}
	if !strings.Contains(w.asked[0], "black and gold") {
		t.Fatal("the writer gets the description")
	}
	if _, err := os.Stat(e.Store.PreviewPath(d.ID, got.Preview[0])); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Cat.Get("probe"); ok {
		t.Fatal("a draft is not in the catalog")
	}
}

func TestARefusedAnswerIsAskedAgainWithTheReason(t *testing.T) {
	bad := answerOf(testManifest, `<img src="https://example.com/x.png">`+movingFragment)
	w := &scriptedWriter{answers: []string{bad, answerOf(testManifest, movingFragment)}}
	e := draftEngine(t, w, nil)
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Attempts != 2 || !strings.Contains(w.asked[1], "external") || !strings.Contains(w.asked[1], "previous answer was refused") {
		t.Fatalf("%+v\n%v", got, w.asked)
	}
}

func TestADraftThatNeverPassesKeepsTheLastFilesAndProblems(t *testing.T) {
	still := `<div style="position:absolute;left:10px;top:10px;width:50px;height:50px;background:var(--accent)"></div><script>window.__partSeek=function(t,b){};</script>`
	w := &scriptedWriter{answers: []string{answerOf(testManifest, still)}}
	e := draftEngine(t, w, nil)
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftFailed || got.Attempts != Attempts || len(got.Problems) == 0 || !strings.Contains(got.Problems[0], "does not move") || !strings.Contains(got.Fragment, "__partSeek") || len(got.Preview) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestAMinorStopsTheDraftAndThePictureIsNotKept(t *testing.T) {
	w := &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment)}}
	e := draftEngine(t, w, func(context.Context, []byte) (string, error) { return "", ErrMinorPicture })
	d, _ := e.Store.Create("a dot", "upload", []byte("jpg"))
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftFailed || !strings.Contains(got.Error, "under 18") || len(w.asked) != 0 {
		t.Fatalf("nothing is written for a minor: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(e.Store.Dir, d.ID, "frame.jpg")); err == nil {
		t.Fatal("the picture is deleted")
	}
}

func TestASentenceAloneNeverAsksTheDescriberAndNoAssistantIsSaid(t *testing.T) {
	e := draftEngine(t, &scriptedWriter{answers: []string{answerOf(testManifest, movingFragment)}}, func(context.Context, []byte) (string, error) {
		t.Fatal("no picture, no description")
		return "", nil
	})
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftReady || got.WithPicture {
		t.Fatalf("%+v", got)
	}
	e.Writer = nil
	d, _ = e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftFailed || !strings.Contains(got.Error, "no assistant") {
		t.Fatalf("%+v", got)
	}
	e.Writer = &scriptedWriter{err: errors.New("down")}
	d, _ = e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftFailed || !strings.Contains(got.Error, "down") {
		t.Fatalf("%+v", got)
	}
}

func TestEditingChecksTheDraftAndApprovingAddsThePartToTheCatalog(t *testing.T) {
	still := `<div style="position:absolute;left:10px;top:10px;width:50px;height:50px"></div><script>window.__partSeek=function(t,b){};</script>`
	e := draftEngine(t, &scriptedWriter{answers: []string{answerOf(testManifest, still)}}, nil)
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(context.Background(), d.ID)
	if _, _, err := e.Approve(context.Background(), d.ID); !errors.Is(err, ErrDraftNotReady) {
		t.Fatalf("a failed draft is not approved: %v", err)
	}
	// A bad edit changes nothing and says why.
	got, probs, err := e.Edit(context.Background(), d.ID, testManifest, `<img src="https://example.com/a.png">`+movingFragment)
	if err != nil || len(probs) == 0 || got.State != DraftFailed || strings.Contains(got.Fragment, "example.com") {
		t.Fatalf("%v %v %+v", err, probs, got)
	}
	// A good edit makes it ready with a preview.
	got, probs, err = e.Edit(context.Background(), d.ID, testManifest, movingFragment)
	if err != nil || len(probs) != 0 || got.State != DraftReady || len(got.Preview) != DraftPreviewFrames {
		t.Fatalf("%v %v %+v", err, probs, got)
	}
	if _, ok := e.Cat.Get("probe"); ok {
		t.Fatal("not in the catalog until approved")
	}
	part, probs, err := e.Approve(context.Background(), d.ID)
	if err != nil || len(probs) != 0 || part.Name != "probe" || part.Source != "user" {
		t.Fatalf("%v %v %+v", err, probs, part)
	}
	if _, ok := e.Cat.Get("probe"); !ok {
		t.Fatal("approved: it is in the catalog")
	}
	if _, err := e.Store.Get(d.ID); !errors.Is(err, ErrDraftNotFound) {
		t.Fatal("the draft is gone")
	}
	// A second draft with the same name cannot be approved over it.
	d2, _ := e.Store.Create("again", "", nil)
	_ = e.Store.SaveFiles(d2.ID, testManifest, movingFragment)
	d2.State, d2.Name = DraftReady, "probe"
	_ = e.Store.Save(d2)
	if _, _, err := e.Approve(context.Background(), d2.ID); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("%v", err)
	}
	if _, err := e.Store.Get(d2.ID); err != nil {
		t.Fatal("a refused approval leaves the draft")
	}
}

func TestADeletedDraftIsNotBroughtBackAndARestartMarksTheRunningOnes(t *testing.T) {
	s := NewDraftStore(t.TempDir())
	a, _ := s.Create("a", "", nil)
	b, _ := s.Create("b", "", nil)
	b.State = DraftReady
	_ = s.Save(b)
	if n := s.MarkInterrupted(); n != 1 {
		t.Fatalf("%d", n)
	}
	if got, _ := s.Get(a.ID); got.State != DraftFailed || !strings.Contains(got.Error, "interrupted") {
		t.Fatalf("%+v", got)
	}
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(a); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := s.Get("../x"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatal(err)
	}
}

func TestACancelledJobWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &cancelWriter{cancel: cancel}
	e := draftEngine(t, w, nil)
	d, _ := e.Store.Create("a dot", "", nil)
	e.Run(ctx, d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftWriting {
		t.Fatalf("a cancelled job leaves the draft as it was: %+v", got)
	}
}

type cancelWriter struct{ cancel context.CancelFunc }

func (w *cancelWriter) Write(context.Context, string, string, int) (string, error) {
	w.cancel()
	return "", errors.New("stopped")
}

func TestAnotherAttemptWritesAgainFromTheSameSentenceAndDescription(t *testing.T) {
	still := `<div style="position:absolute;left:10px;top:10px;width:50px;height:50px"></div><script>window.__partSeek=function(t,b){};</script>`
	w := &scriptedWriter{answers: []string{answerOf(testManifest, still), answerOf(testManifest, still), answerOf(testManifest, still), answerOf(testManifest, movingFragment)}}
	var described int
	e := draftEngine(t, w, func(context.Context, []byte) (string, error) { described++; return "gold", nil })
	d, _ := e.Store.Create("a dot", "upload", []byte("jpg"))
	e.Run(context.Background(), d.ID)
	if got, _ := e.Store.Get(d.ID); got.State != DraftFailed {
		t.Fatalf("%+v", got)
	}
	if err := e.Restart(d.ID); err != nil {
		t.Fatal(err)
	}
	e.Run(context.Background(), d.ID)
	got, _ := e.Store.Get(d.ID)
	if got.State != DraftReady || got.Attempts != 1 || described != 1 || !strings.Contains(w.asked[len(w.asked)-1], "gold") {
		t.Fatalf("the picture is described once: %+v %d", got, described)
	}
	if err := e.Restart(d.ID); err == nil {
		t.Fatal("a ready draft is not tried again")
	}
	// The picture of a refused draft is gone: it cannot be tried again.
	m := draftEngine(t, w, func(context.Context, []byte) (string, error) { return "", ErrMinorPicture })
	md, _ := m.Store.Create("a dot", "upload", []byte("jpg"))
	m.Run(context.Background(), md.ID)
	if err := m.Restart(md.ID); err == nil || !strings.Contains(err.Error(), "deleted") {
		t.Fatalf("%v", err)
	}
}
