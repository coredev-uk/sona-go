// Package id3test builds ID3v2 tags for tests.
package id3test

import "encoding/binary"

// Tag returns an ID3v2 tag of version 2.major holding frames (made with
// Frame for the same version), with some padding.
func Tag(major byte, frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	body = append(body, make([]byte, 32)...) // padding
	return append(append([]byte{'I', 'D', '3', major, 0, 0}, syncsafe(len(body))...), body...)
}

// Frame returns an ID3v2.major frame (three-character IDs in v2.2).
func Frame(major byte, id string, body []byte) []byte {
	switch major {
	case 2:
		n := len(body)
		return append([]byte{id[0], id[1], id[2], byte(n >> 16), byte(n >> 8), byte(n)}, body...)
	case 3:
		return append(append([]byte(id), binary.BigEndian.AppendUint32(nil, uint32(len(body)))...), append([]byte{0, 0}, body...)...)
	}
	return append(append([]byte(id), syncsafe(len(body))...), append([]byte{0, 0}, body...)...)
}

// Text returns a text frame's body holding s in UTF-8 (encoding 3).
func Text(s string) []byte { return append([]byte{3}, s...) }

func syncsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
}
