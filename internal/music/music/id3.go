package music

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// Tags are what an exported mp3 carries so that any player shows the song: its title, artist and album,
// the style it was made from, the lyrics sung and a cover picture.
type Tags struct {
	Title   string
	Artist  string
	Album   string
	Comment string
	Lyrics  string
	// Track is the 1-based position of the song in the project; 0 omits it.
	Track int
	// Cover is the picture to embed as the front cover and its MIME type; nil embeds none.
	Cover     []byte
	CoverMIME string
}

// WithID3 returns the mp3 with an ID3v2.3 tag in front: the audio bytes are untouched (they are not encoded again),
// and a tag the file already had at its start is replaced. ID3v2.3 is what every player reads.
func WithID3(mp3 []byte, t Tags) []byte {
	var frames bytes.Buffer
	text := func(id, value string) {
		if value != "" {
			writeFrame(&frames, id, append([]byte{1}, utf16le(value)...))
		}
	}
	text("TIT2", t.Title)
	text("TPE1", t.Artist)
	text("TALB", t.Album)
	if t.Track > 0 {
		text("TRCK", itoa(t.Track))
	}
	if t.Comment != "" {
		writeFrame(&frames, "COMM", withLanguage(t.Comment))
	}
	if t.Lyrics != "" {
		writeFrame(&frames, "USLT", withLanguage(t.Lyrics))
	}
	if len(t.Cover) > 0 {
		mime := t.CoverMIME
		if mime == "" {
			mime = "image/png"
		}
		body := []byte{0} // ISO-8859-1 for the MIME type and the (empty) description
		body = append(body, mime...)
		body = append(body, 0, 3, 0) // the MIME's terminator, picture type 3 (front cover), an empty description
		body = append(body, t.Cover...)
		writeFrame(&frames, "APIC", body)
	}
	if frames.Len() == 0 {
		return mp3
	}
	var out bytes.Buffer
	out.WriteString("ID3")
	out.Write([]byte{3, 0, 0}) // version 2.3.0, no flags
	out.Write(syncsafe(frames.Len()))
	out.Write(frames.Bytes())
	out.Write(stripID3(mp3))
	return out.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// utf16le is the text as UTF-16 with a byte-order mark, the encoding ID3v2.3 uses for anything beyond Latin-1.
func utf16le(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(units))
	b[0], b[1] = 0xFF, 0xFE
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// withLanguage is the body of a comment or lyrics frame: UTF-16, language "eng", an empty description, the text.
func withLanguage(value string) []byte {
	body := []byte{1, 'e', 'n', 'g', 0xFF, 0xFE, 0, 0} // description: a BOM and the 2-byte terminator
	return append(body, utf16le(value)...)
}

func writeFrame(w *bytes.Buffer, id string, body []byte) {
	w.WriteString(id)
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(body))) // plain big-endian in ID3v2.3, not syncsafe
	w.Write(size[:])
	w.Write([]byte{0, 0})
	w.Write(body)
}

// syncsafe writes a 28-bit size with 7 bits per byte.
func syncsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

// stripID3 drops an ID3v2 tag at the start of the file, if there is one.
func stripID3(b []byte) []byte {
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		size := int(b[6]&0x7F)<<21 | int(b[7]&0x7F)<<14 | int(b[8]&0x7F)<<7 | int(b[9]&0x7F)
		if end := 10 + size; end <= len(b) {
			return b[end:]
		}
	}
	return b
}
