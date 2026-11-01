package id3

import (
	"bytes"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/playback/id3/id3test"
)

func TestLen(t *testing.T) {
	tag := id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text("x")))
	footer := slices.Clone(tag)
	footer[5] |= 0x10
	badSize := slices.Clone(tag)
	badSize[7] = 0x80
	for _, c := range []struct {
		name string
		b    []byte
		n    int
		ok   bool
	}{
		{"tag", tag, len(tag), true},
		{"tag and audio", append(slices.Clone(tag), 0xff, 0xfb, 0x90), len(tag), true},
		{"header only", tag[:10], len(tag), true},
		{"v2.4 footer", footer, len(tag) + 10, true},
		{"v2.3 flag is no footer", func() []byte { b := id3test.Tag(3); b[5] |= 0x10; return b }(), len(id3test.Tag(3)), true},
		{"empty", nil, 0, false},
		{"partial magic", []byte("ID"), 0, false},
		{"partial header", tag[:9], 0, false},
		{"audio", []byte{0xff, 0xfb, 0x90, 0x44}, 0, true},
		{"short audio", []byte{0xff}, 0, true},
		{"unknown version", append([]byte("ID3\x05\x00\x00"), 0, 0, 0, 1), 0, true},
		{"version 1", append([]byte("ID3\x01\x00\x00"), 0, 0, 0, 1), 0, true},
		{"size not syncsafe", badSize, 0, true},
	} {
		n, ok := Len(c.b)
		if n != c.n || ok != c.ok {
			t.Errorf("%s: Len = %d, %v; want %d, %v", c.name, n, ok, c.n, c.ok)
		}
	}
}

type frame struct{ id, body string }

func frames(t *testing.T, b []byte) ([]frame, bool) {
	t.Helper()
	var out []frame
	ok := Frames(b, func(id string, body []byte) bool {
		out = append(out, frame{id, string(body)})
		return true
	})
	return out, ok
}

func TestFrames(t *testing.T) {
	want := []frame{{"TIT2", "\x03Song"}, {"TPE1", "\x03Artist"}}
	for _, major := range []byte{3, 4} {
		tag := id3test.Tag(major, id3test.Frame(major, "TIT2", id3test.Text("Song")), id3test.Frame(major, "TPE1", id3test.Text("Artist")))
		got, ok := frames(t, tag)
		if !ok || !slices.Equal(got, want) {
			t.Errorf("v2.%d: %q %v", major, got, ok)
		}
	}
	got, ok := frames(t, id3test.Tag(2, id3test.Frame(2, "TT2", id3test.Text("Song")), id3test.Frame(2, "TP1", id3test.Text("Artist"))))
	if !ok || !slices.Equal(got, []frame{{"TT2", "\x03Song"}, {"TP1", "\x03Artist"}}) {
		t.Errorf("v2.2: %q %v", got, ok)
	}
	// A frame larger than 127 bytes: v2.3 sizes are plain, v2.4 syncsafe.
	long := bytes.Repeat([]byte("a"), 300)
	for _, major := range []byte{2, 3, 4} {
		id := "PRIV"
		if major == 2 {
			id = "PRV"
		}
		got, ok := frames(t, id3test.Tag(major, id3test.Frame(major, id, long)))
		if !ok || len(got) != 1 || got[0].body != string(long) {
			t.Errorf("v2.%d long frame: %d frames, %v", major, len(got), ok)
		}
	}
}

func TestFramesStop(t *testing.T) {
	tag := id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text("a")), id3test.Frame(4, "TPE1", id3test.Text("b")))
	var ids []string
	ok := Frames(tag, func(id string, _ []byte) bool {
		ids = append(ids, id)
		return false
	})
	if !ok || !slices.Equal(ids, []string{"TIT2"}) {
		t.Fatalf("%v %v", ids, ok)
	}
}

// withExtendedHeader returns a tag of version 2.major whose body starts
// with ext, flagged as an extended header.
func withExtendedHeader(major byte, ext []byte, frames ...[]byte) []byte {
	plain := id3test.Tag(major, frames...)
	body := append(slices.Clone(ext), plain[10:]...)
	tag := id3test.Tag(major)[:10]
	n := len(body)
	tag[5] = 0x40
	tag[6], tag[7], tag[8], tag[9] = byte(n>>21&0x7f), byte(n>>14&0x7f), byte(n>>7&0x7f), byte(n&0x7f)
	return append(tag, body...)
}

func TestFramesExtendedHeader(t *testing.T) {
	f3 := id3test.Frame(3, "TIT2", id3test.Text("v3"))
	f4 := id3test.Frame(4, "TIT2", id3test.Text("v4"))
	// v2.3: the size excludes its own 4 bytes; v2.4: syncsafe, includes them.
	v3 := withExtendedHeader(3, []byte{0, 0, 0, 6, 0, 0, 0, 0, 0, 0}, f3)
	v4 := withExtendedHeader(4, []byte{0, 0, 0, 6, 1, 0}, f4)
	for name, c := range map[string]struct {
		tag  []byte
		want string
	}{"v2.3": {v3, "\x03v3"}, "v2.4": {v4, "\x03v4"}} {
		got, ok := frames(t, c.tag)
		if !ok || len(got) != 1 || got[0] != (frame{"TIT2", c.want}) {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	if _, ok := frames(t, withExtendedHeader(4, []byte{0, 0, 0x7f, 0x7f}, f4)); ok {
		t.Error("extended header longer than the tag must fail")
	}
}

func TestFramesUnreadable(t *testing.T) {
	tag := id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text("Song")))
	unsync := slices.Clone(tag)
	unsync[5] |= 0x80
	// A frame claiming more bytes than the tag holds.
	overrun := id3test.Tag(3, append([]byte("TIT2"), 0, 0, 1, 0, 0, 0, 3, 'x'))
	for name, b := range map[string][]byte{
		"not a tag":      []byte("not a tag at all"),
		"truncated":      tag[:len(tag)-1],
		"unsynchronised": unsync,
		"frame overrun":  overrun,
		"empty":          nil,
	} {
		if got, ok := frames(t, b); ok {
			t.Errorf("%s: read %q", name, got)
		}
	}
	// Padding ends the frames.
	if got, ok := frames(t, id3test.Tag(4)); !ok || len(got) != 0 {
		t.Errorf("padding only: %q %v", got, ok)
	}
}

func TestText(t *testing.T) {
	for _, c := range []struct {
		body []byte
		want string
	}{
		{nil, ""},
		{[]byte{3}, ""},
		{id3test.Text("Song"), "Song"},
		{id3test.Text(" Song \x00"), "Song"},
		{id3test.Text("A\x00B\x00\x00 C "), "A, B, C"},
		{append([]byte{0, 'C', 0xe9}, "line"...), "Céline"},
		{[]byte{1, 0xff, 0xfe, 'H', 0, 'i', 0, 0, 0}, "Hi"},
		{[]byte{1, 0xfe, 0xff, 0, 'H', 0, 'i'}, "Hi"},
		{[]byte{2, 0, 'H', 0, 0xe9}, "Hé"},
		{[]byte{1, 0x3d, 0xd8, 0xb5, 0xdc}, "\U0001f4b5"}, // no BOM: little-endian, a surrogate pair
	} {
		if got := Text(c.body); got != c.want {
			t.Errorf("Text(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestDecode(t *testing.T) {
	for _, c := range []struct {
		enc  byte
		b    []byte
		want string
	}{
		{0, []byte("Mot\xf6rhead\x00"), "Motörhead"},
		{3, []byte("Motörhead\x00\x00"), "Motörhead"},
		{3, []byte("bad\xff"), "bad\uFFFD"},
		{1, []byte{0xff, 0xfe, 'a', 0, 'b'}, "a"}, // odd trailing byte dropped
		{2, []byte{0xfe, 0xff, 0, 'a'}, "a"},      // a BOM overrides the encoding's byte order
		{2, []byte{0xff, 0xfe, 'a', 0}, "a"},
		{9, []byte("x"), "x"}, // unknown encodings read as ISO-8859-1
	} {
		if got := Decode(c.enc, c.b); got != c.want {
			t.Errorf("Decode(%d, %q) = %q, want %q", c.enc, c.b, got, c.want)
		}
	}
}
