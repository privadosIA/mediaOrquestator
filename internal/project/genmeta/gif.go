package genmeta

import "strings"

// fromGIF reads the comment extensions of a GIF. A comment is only used when
// it is a generation text (it has the "Steps:" line): a GIF comment such as
// "Created with GIMP" says nothing about how the picture was made.
func fromGIF(data []byte) Metadata {
	if len(data) < 13 {
		return Metadata{}
	}
	pos := 13
	flags := data[10]
	if flags&0x80 != 0 {
		pos += 3 * (1 << (int(flags&7) + 1)) // global color table
	}
	var comments []string
	for pos < len(data) {
		switch data[pos] {
		case 0x3B: // trailer
			return fromComments(comments)
		case 0x21: // extension
			if pos+2 > len(data) {
				return fromComments(comments)
			}
			label := data[pos+1]
			pos += 2
			var buf []byte
			var ok bool
			buf, pos, ok = readSubBlocks(data, pos, label == 0xFE)
			if !ok {
				return fromComments(comments)
			}
			if label == 0xFE {
				comments = append(comments, string(buf))
			}
		case 0x2C: // image descriptor
			if pos+10 > len(data) {
				return fromComments(comments)
			}
			lf := data[pos+9]
			pos += 10
			if lf&0x80 != 0 {
				pos += 3 * (1 << (int(lf&7) + 1))
			}
			pos++ // LZW minimum code size
			var ok bool
			_, pos, ok = readSubBlocks(data, pos, false)
			if !ok {
				return fromComments(comments)
			}
		default:
			return fromComments(comments)
		}
	}
	return fromComments(comments)
}

// readSubBlocks reads (or, without keep, skips) the length-prefixed blocks
// that end at a zero length. It stops at a truncated file or an oversized value.
func readSubBlocks(data []byte, pos int, keep bool) ([]byte, int, bool) {
	var out []byte
	for pos < len(data) {
		n := int(data[pos])
		pos++
		if n == 0 {
			return out, pos, true
		}
		if pos+n > len(data) {
			return out, pos, false
		}
		if keep {
			if len(out)+n > MaxValue*2 {
				return out, pos, false
			}
			out = append(out, data[pos:pos+n]...)
		}
		pos += n
	}
	return out, pos, false
}

func fromComments(comments []string) Metadata {
	for _, c := range comments {
		if !strings.Contains(c, "Steps: ") && !strings.HasPrefix(strings.TrimSpace(c), "{") {
			continue
		}
		if m := FromText(c); !m.Empty() {
			return m
		}
	}
	return Metadata{}
}
