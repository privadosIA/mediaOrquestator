package genmeta

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"strings"
)

// fromPNG walks the chunks of a PNG and reads its text ones. The first chunk
// that yields settings wins, except that a "parameters" text and a ComfyUI
// "prompt" are both tried, parameters first.
func fromPNG(data []byte) Metadata {
	var params, comfy Metadata
	pos := 8
	for pos+8 <= len(data) {
		n := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		start := pos + 8
		end := start + n
		if n < 0 || end < start || end > len(data) {
			break // truncated or lying about its size: stop with what was valid
		}
		if typ == "tEXt" || typ == "iTXt" || typ == "zTXt" {
			if n <= MaxValue*2 {
				if key, text, ok := pngText(typ, data[start:end]); ok {
					switch key {
					case "parameters":
						if params.Empty() {
							params = FromText(text)
						}
					case "prompt":
						if comfy.Empty() {
							comfy = FromText(text)
						}
					}
				}
			}
		}
		if typ == "IEND" {
			break
		}
		pos = end + 4 // data + CRC
	}
	if !params.Empty() {
		return params
	}
	return comfy
}

// pngText decodes one text chunk into its keyword and text.
func pngText(typ string, b []byte) (key, text string, ok bool) {
	i := bytes.IndexByte(b, 0)
	if i <= 0 || i > 79 {
		return "", "", false
	}
	key = string(b[:i])
	rest := b[i+1:]
	switch typ {
	case "tEXt":
		return key, latin1OrUTF8(rest), true
	case "zTXt":
		if len(rest) < 1 || rest[0] != 0 {
			return "", "", false
		}
		out, ok := inflate(rest[1:])
		return key, latin1OrUTF8(out), ok
	default: // iTXt: flag, method, language\0, translated keyword\0, text
		if len(rest) < 2 {
			return "", "", false
		}
		compressed, method := rest[0], rest[1]
		rest = rest[2:]
		for k := 0; k < 2; k++ { // language tag, translated keyword
			j := bytes.IndexByte(rest, 0)
			if j < 0 {
				return "", "", false
			}
			rest = rest[j+1:]
		}
		if compressed == 1 {
			if method != 0 {
				return "", "", false
			}
			out, ok := inflate(rest)
			return key, strings.ToValidUTF8(string(out), ""), ok
		}
		return key, strings.ToValidUTF8(string(rest), ""), true
	}
}

// inflate decompresses zlib data, refusing anything that expands past the bound.
func inflate(b []byte) ([]byte, bool) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, false
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, MaxDecompressed+1))
	if err != nil || len(out) > MaxDecompressed {
		return nil, false
	}
	return out, true
}

// latin1OrUTF8 is how tEXt is read in practice: the spec says Latin-1, but
// stable-diffusion.cpp and A1111 write UTF-8.
func latin1OrUTF8(b []byte) string {
	s := string(b)
	if isValidUTF8(b) {
		return s
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func isValidUTF8(b []byte) bool { return strings.ToValidUTF8(string(b), "\x00") == string(b) }
