package describe

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodLook = "Dark navy backgrounds with neon green accents, a bold monospace typeface set in large capitals, minimal terminal-like screens with a thin grid, text that types in letter by letter with a soft glow around it."

func lookFrames() [][]byte { return [][]byte{[]byte("a"), []byte("b"), []byte("c")} }

func TestDescribeLookWritesTheLookOfTheFrames(t *testing.T) {
	f := newFake()
	f.describeReply = goodLook
	f.mergeReply = "unused"
	d := f.server(t)
	look, err := d.DescribeLook(context.Background(), lookFrames(), Options{})
	if err != nil || look.Text != goodLook || look.Model != DefaultModel {
		t.Fatalf("%v %+v", err, look)
	}
	// An age check and a description for each frame, then the merge, and the prompt asks for the look.
	var checks, looks, merges int
	for _, c := range f.chats {
		sys := c["messages"].([]any)[0].(map[string]any)["content"].(string)
		switch {
		case strings.HasPrefix(sys, "You check a picture"):
			checks++
		case strings.HasPrefix(sys, "You describe the visual look"):
			looks++
		case strings.HasPrefix(sys, "You combine the looks"):
			merges++
		}
	}
	if checks != 3 || looks != 3 || merges != 1 {
		t.Fatalf("checks %d looks %d merges %d", checks, looks, merges)
	}
	if len(f.unloaded) == 0 {
		t.Fatal("the model is unloaded afterwards")
	}
}

func TestDescribeLookStopsAtAMinorAndSaysWhichFrame(t *testing.T) {
	f := newFake()
	f.age = `{"people":1,"minor":true,"unclear":false}`
	d := f.server(t)
	_, err := d.DescribeLook(context.Background(), lookFrames(), Options{})
	var minor *MinorFrameError
	if !errors.Is(err, ErrMinor) || !errors.As(err, &minor) || minor.Frame != 0 {
		t.Fatalf("%v", err)
	}
	for _, c := range f.chats {
		if strings.HasPrefix(c["messages"].([]any)[0].(map[string]any)["content"].(string), "You describe the visual look") {
			t.Fatal("nothing is described when a frame is refused")
		}
	}
}

func TestDescribeLookReportsARefusalAndNeverInventsText(t *testing.T) {
	f := newFake()
	f.describeReply = "I cannot help with that request."
	d := f.server(t)
	look, err := d.DescribeLook(context.Background(), lookFrames(), Options{})
	if !errors.Is(err, ErrRefused) || look.Text != "" {
		t.Fatalf("%v %+v", err, look)
	}
}

func TestDescribeLookWithNoModelInstalledSaysSo(t *testing.T) {
	f := newFake()
	f.installed = map[string]bool{"qwen3:8b": false}
	d := f.server(t)
	if _, err := d.DescribeLook(context.Background(), lookFrames(), Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}

func TestDescribeLookKeepsTheFirstFrameWhenTheMergeFails(t *testing.T) {
	f := newFake()
	f.describeReply = goodLook
	f.mergeReply = "no"
	d := f.server(t)
	look, err := d.DescribeLook(context.Background(), lookFrames(), Options{})
	if err != nil || look.Text != goodLook {
		t.Fatalf("a merge that is not a description leaves a real one: %v %+v", err, look)
	}
}

func TestDescribePictureIsOneFrameWithTheSameSafeguards(t *testing.T) {
	f := newFake()
	f.describeReply = goodLook
	d := f.server(t)
	look, err := d.DescribePicture(context.Background(), []byte("p"), Options{})
	if err != nil || look.Text != goodLook {
		t.Fatalf("%v %+v", err, look)
	}
	var checks, looks, merges int
	for _, c := range f.chats {
		sys := c["messages"].([]any)[0].(map[string]any)["content"].(string)
		switch {
		case strings.HasPrefix(sys, "You check a picture"):
			checks++
		case strings.HasPrefix(sys, "You describe the visual look"):
			looks++
		case strings.HasPrefix(sys, "You combine the looks"):
			merges++
		}
	}
	if checks != 1 || looks != 1 || merges != 0 {
		t.Fatalf("one age check, one description, no merge: %d %d %d", checks, looks, merges)
	}
	f = newFake()
	f.age = `{"people":1,"minor":false,"unclear":true}`
	var minor *MinorFrameError
	if _, err := f.server(t).DescribePicture(context.Background(), []byte("p"), Options{}); !errors.As(err, &minor) || minor.Frame != 0 {
		t.Fatalf("%v", err)
	}
	f = newFake()
	f.describeReply = "no."
	if _, err := f.server(t).DescribePicture(context.Background(), []byte("p"), Options{}); !errors.Is(err, ErrRefused) {
		t.Fatalf("%v", err)
	}
	f = newFake()
	f.installed = map[string]bool{"qwen3:8b": false}
	if _, err := f.server(t).DescribePicture(context.Background(), []byte("p"), Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}
