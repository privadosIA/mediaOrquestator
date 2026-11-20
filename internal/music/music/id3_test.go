package music

import (
	"bytes"
	"testing"
	"unicode/utf16"
)

func TestWithID3_PutsTheTagInFrontAndLeavesTheAudioUntouched(t *testing.T) {
	audio := bytes.Repeat([]byte{0xFF, 0xFB, 0x90, 0x00}, 100)
	out := WithID3(audio, Tags{Title: "Noche de verano ☀", Artist: "Yo", Album: "Canciones", Track: 3, Comment: "pop, guitarra", Lyrics: "[Verse]\nhola"})
	if string(out[:3]) != "ID3" || out[3] != 3 {
		t.Fatalf("header = %q", out[:6])
	}
	if !bytes.HasSuffix(out, audio) {
		t.Error("the audio must follow the tag byte for byte")
	}
	size := int(out[6])<<21 | int(out[7])<<14 | int(out[8])<<7 | int(out[9])
	if 10+size+len(audio) != len(out) {
		t.Errorf("the size in the header (%d) does not match the tag", size)
	}
	for _, id := range []string{"TIT2", "TPE1", "TALB", "TRCK", "COMM", "USLT"} {
		if !bytes.Contains(out[:10+size], []byte(id)) {
			t.Errorf("frame %s missing", id)
		}
	}
	if bytes.Contains(out[:10+size], []byte("APIC")) {
		t.Error("no cover was given")
	}
	// the title is UTF-16 with a byte-order mark and keeps its accents and symbols
	units := utf16.Encode([]rune("Noche de verano ☀"))
	want := []byte{0xFF, 0xFE}
	for _, u := range units {
		want = append(want, byte(u), byte(u>>8))
	}
	if !bytes.Contains(out[:10+size], want) {
		t.Error("the title is not written as UTF-16")
	}
}

func TestWithID3_CoverIsAFrontPictureAndAnOldTagIsReplaced(t *testing.T) {
	audio := []byte("AUDIOAUDIO")
	first := WithID3(audio, Tags{Title: "old"})
	second := WithID3(first, Tags{Title: "new", Cover: []byte("PNGDATA"), CoverMIME: "image/png"})
	if bytes.Contains(second, []byte("old")) || !bytes.HasSuffix(second, audio) {
		t.Error("the previous tag must be replaced, and the audio kept")
	}
	if !bytes.Contains(second, []byte("APIC")) || !bytes.Contains(second, []byte("image/png\x00\x03\x00PNGDATA")) {
		t.Errorf("the cover frame is wrong: %q", second[:80])
	}
}

func TestWithID3_NothingToWriteLeavesTheFileAlone(t *testing.T) {
	audio := []byte("AUDIO")
	if got := WithID3(audio, Tags{}); !bytes.Equal(got, audio) {
		t.Error("an empty tag must not change the file")
	}
}
