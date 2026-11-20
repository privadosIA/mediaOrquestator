package genmeta

import (
	"bytes"
	"encoding/binary"
	"html"
	"regexp"
	"strings"
	"unicode/utf16"
)

// fromWebP reads the EXIF (UserComment, ImageDescription) and the XMP
// (dc:description, exif:UserComment) chunks of a WebP: A1111 writes its
// parameters there.
func fromWebP(data []byte) Metadata {
	pos := 12
	for pos+8 <= len(data) {
		fourcc := string(data[pos : pos+4])
		n := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		end := start + n
		if n < 0 || end < start || end > len(data) {
			return Metadata{}
		}
		switch fourcc {
		case "EXIF":
			if n <= MaxValue*4 {
				for _, t := range exifTexts(data[start:end]) {
					if m := FromText(t); !m.Empty() {
						return m
					}
				}
			}
		case "XMP ":
			if n <= MaxValue*4 {
				for _, t := range xmpTexts(data[start:end]) {
					if m := FromText(t); !m.Empty() {
						return m
					}
				}
			}
		}
		pos = end + n%2 // chunks are padded to an even size
	}
	return Metadata{}
}

var xmpRe = regexp.MustCompile(`(?s)<(?:dc:description|exif:UserComment)[^>]*>(?:\s*<rdf:Alt>\s*<rdf:li[^>]*>)?([^<]*)`)

func xmpTexts(b []byte) []string {
	var out []string
	for _, m := range xmpRe.FindAllSubmatch(b, -1) {
		if t := strings.TrimSpace(html.UnescapeString(string(m[1]))); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// exifTexts returns the text of ImageDescription (0x010E) in the first IFD
// and of UserComment (0x9286) in the Exif IFD it points to (0x8769).
func exifTexts(b []byte) []string {
	b = bytes.TrimPrefix(b, []byte("Exif\x00\x00"))
	if len(b) < 8 {
		return nil
	}
	var bo binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil
	}
	if bo.Uint16(b[2:4]) != 42 {
		return nil
	}
	var out []string
	exifIFD := -1
	for _, e := range ifdEntries(b, int(bo.Uint32(b[4:8])), bo) {
		switch e.tag {
		case 0x010E:
			if t, ok := e.text(b, bo); ok {
				out = append(out, t)
			}
		case 0x8769:
			exifIFD = int(e.valueOrOffset)
		}
	}
	if exifIFD >= 0 {
		for _, e := range ifdEntries(b, exifIFD, bo) {
			if e.tag == 0x9286 {
				if t, ok := e.userComment(b, bo); ok {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

type ifdEntry struct {
	tag, typ      uint16
	count         uint32
	valueOrOffset uint32
	inline        []byte
}

func ifdEntries(b []byte, off int, bo binary.ByteOrder) []ifdEntry {
	if off < 0 || off+2 > len(b) {
		return nil
	}
	n := int(bo.Uint16(b[off : off+2]))
	if n > 256 {
		return nil
	}
	var out []ifdEntry
	for i := 0; i < n; i++ {
		p := off + 2 + i*12
		if p+12 > len(b) {
			break
		}
		out = append(out, ifdEntry{
			tag:           bo.Uint16(b[p : p+2]),
			typ:           bo.Uint16(b[p+2 : p+4]),
			count:         bo.Uint32(b[p+4 : p+8]),
			valueOrOffset: bo.Uint32(b[p+8 : p+12]),
			inline:        b[p+8 : p+12],
		})
	}
	return out
}

// raw returns the bytes of the entry's value (1 byte per count, as for ASCII
// and UNDEFINED), inline when it fits in four bytes.
func (e ifdEntry) raw(b []byte) ([]byte, bool) {
	if e.count > MaxValue*2 {
		return nil, false
	}
	n := int(e.count)
	if n <= 4 {
		return e.inline[:n], true
	}
	off := int(e.valueOrOffset)
	if off < 0 || off+n > len(b) {
		return nil, false
	}
	return b[off : off+n], true
}

func (e ifdEntry) text(b []byte, _ binary.ByteOrder) (string, bool) {
	v, ok := e.raw(b)
	if !ok {
		return "", false
	}
	return strings.TrimRight(strings.ToValidUTF8(string(v), ""), "\x00"), true
}

// userComment decodes the 8-byte character code followed by the text.
func (e ifdEntry) userComment(b []byte, bo binary.ByteOrder) (string, bool) {
	v, ok := e.raw(b)
	if !ok || len(v) < 8 {
		return "", false
	}
	code, body := string(v[:8]), v[8:]
	switch {
	case strings.HasPrefix(code, "UNICODE"):
		// UTF-16 in the file's byte order (A1111 writes big endian).
		order := bo
		if len(body) >= 2 && body[0] == 0xFF && body[1] == 0xFE {
			order, body = binary.LittleEndian, body[2:]
		} else if len(body) >= 2 && body[0] == 0xFE && body[1] == 0xFF {
			order, body = binary.BigEndian, body[2:]
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			u[i] = order.Uint16(body[2*i:])
		}
		return strings.TrimRight(string(utf16.Decode(u)), "\x00"), true
	case strings.HasPrefix(code, "ASCII"), strings.HasPrefix(code, "\x00\x00\x00\x00"):
		return strings.TrimRight(strings.ToValidUTF8(string(body), ""), "\x00"), true
	}
	return "", false
}
