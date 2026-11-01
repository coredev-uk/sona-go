// Package id3 reads ID3v2 tags (versions 2.2 to 2.4) at the start of a
// byte slice: their length, their frames and the text in them. Sona reads
// the Sound Check comment of uploaded MP3s (internal/playback/soundcheck)
// and the title and artist local broadcasters put in their streams
// (internal/playback/broadcast). Unsynchronised tags are not read.
package id3

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// headerLen is the length of a tag's header (and of its v2.4 footer).
const headerLen = 10

// Len returns the length of the ID3v2 tag at the start of b: header, body
// and footer. It returns 0 when b does not start with a tag, and ok false
// when b is too short to tell.
func Len(b []byte) (n int, ok bool) {
	if len(b) < headerLen {
		// A tag may start here: more bytes are needed to know.
		return 0, !bytes.HasPrefix([]byte("ID3"), b[:min(len(b), 3)])
	}
	if string(b[:3]) != "ID3" || b[3] < 2 || b[3] > 4 || !syncsafeOK(b[6:10]) {
		return 0, true
	}
	n = headerLen + syncsafe(b[6:10])
	if b[3] == 4 && b[5]&0x10 != 0 {
		n += headerLen // footer
	}
	return n, true
}

// Frames calls fn with the ID and body of each frame of the tag at the
// start of b (three-character IDs in v2.2) until fn returns false. It
// reports false when b does not hold a whole, readable tag.
func Frames(b []byte, fn func(id string, body []byte) bool) bool {
	n, ok := Len(b)
	if !ok || n == 0 || n > len(b) {
		return false
	}
	major, flags := b[3], b[5]
	if flags&0x80 != 0 { // unsynchronised
		return false
	}
	tag := b[headerLen : headerLen+syncsafe(b[6:10])]
	if flags&0x40 != 0 && major >= 3 { // skip the extended header
		if len(tag) < 4 {
			return false
		}
		ext := int(binary.BigEndian.Uint32(tag))
		if major == 4 {
			ext = syncsafe(tag[:4])
		} else {
			ext += 4
		}
		if ext < 0 || ext > len(tag) {
			return false
		}
		tag = tag[ext:]
	}
	idLen, hdr := 4, 10
	if major == 2 {
		idLen, hdr = 3, 6
	}
	for len(tag) >= hdr && tag[0] != 0 {
		id := string(tag[:idLen])
		var size int
		switch major {
		case 2:
			size = int(tag[3])<<16 | int(tag[4])<<8 | int(tag[5])
		case 3:
			size = int(binary.BigEndian.Uint32(tag[4:8]))
		default:
			size = syncsafe(tag[4:8])
		}
		if size < 0 || hdr+size > len(tag) {
			return false
		}
		if !fn(id, tag[hdr:hdr+size]) {
			return true
		}
		tag = tag[hdr+size:]
	}
	return true
}

// Text returns the text of a text frame's body (an encoding byte, then
// the text). Several values (ID3v2.4 separates them with NUL) are joined
// with ", ".
func Text(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var vals []string
	for v := range strings.SplitSeq(Decode(body[0], body[1:]), "\x00") {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	return strings.Join(vals, ", ")
}

// Decode returns b, ID3 text in encoding enc (0 ISO-8859-1, 1 UTF-16
// with a byte order mark, 2 UTF-16BE, 3 UTF-8), as Go text, without
// trailing NULs.
func Decode(enc byte, b []byte) string {
	switch enc {
	case 1, 2:
		be := enc == 2
		if len(b) >= 2 {
			switch {
			case b[0] == 0xff && b[1] == 0xfe:
				be, b = false, b[2:]
			case b[0] == 0xfe && b[1] == 0xff:
				be, b = true, b[2:]
			}
		}
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			if be {
				u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
			} else {
				u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
			}
		}
		return strings.TrimRight(string(utf16.Decode(u)), "\x00")
	case 3:
		return strings.TrimRight(strings.ToValidUTF8(string(b), "\uFFFD"), "\x00")
	default:
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return strings.TrimRight(string(r), "\x00")
	}
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

// syncsafeOK reports whether b is a valid syncsafe integer (no byte with
// its top bit set), which tells a tag header from audio that happens to
// start with "ID3".
func syncsafeOK(b []byte) bool {
	return b[0]|b[1]|b[2]|b[3] < 0x80
}
