package describe

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSession_OneModelManyQuestionsAndTheSafeguardsOfDescribe(t *testing.T) {
	f := newFake()
	f.describeReply = `{"subject":"typography"}`
	d := f.server(t)
	s, err := d.OpenSession(context.Background(), Options{}, 10)
	if err != nil || s.Model() != DefaultModel {
		t.Fatalf("%v", err)
	}
	if err := s.Check(context.Background(), []byte("a")); err != nil {
		t.Fatalf("an adult is fine: %v", err)
	}
	out, err := s.Ask(context.Background(), "Say JSON.", "two pictures", [][]byte{[]byte("a"), []byte("b")}, true, 100)
	if err != nil || !strings.Contains(out, "typography") {
		t.Fatalf("%q %v", out, err)
	}
	s.Close()
	if len(f.unloaded) != 1 {
		t.Fatalf("the model is given back once: %v", f.unloaded)
	}
}

func TestSession_AMinorAndARefusalAreErrorsNeverReplaced(t *testing.T) {
	f := newFake()
	f.age = `{"people":1,"minor":false,"unclear":true}`
	f.describeReply = "I cannot help with that request."
	d := f.server(t)
	s, err := d.OpenSession(context.Background(), Options{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Check(context.Background(), []byte("a")); !errors.Is(err, ErrMinor) {
		t.Fatalf("an age that cannot be confirmed stops: %v", err)
	}
	if _, err := s.Ask(context.Background(), "x", "y", [][]byte{[]byte("a")}, false, 50); !errors.Is(err, ErrRefused) {
		t.Fatalf("a refusal is reported: %v", err)
	}
}

func TestSession_AJobOverTheCallCapIsRefusedBeforeAnyCall(t *testing.T) {
	f := newFake()
	d := f.server(t)
	d.MaxCalls = 5
	if _, err := d.OpenSession(context.Background(), Options{}, 120); !errors.Is(err, ErrTooManyCalls) {
		t.Fatalf("%v", err)
	}
	if len(f.chats) != 0 {
		t.Fatalf("no call was made: %d", len(f.chats))
	}
}
