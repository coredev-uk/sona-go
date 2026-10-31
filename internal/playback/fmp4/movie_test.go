package fmp4

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mp4Box builds a box of typ around body.
func mp4Box(typ string, body ...[]byte) []byte {
	var b []byte
	for _, p := range body {
		b = append(b, p...)
	}
	return append(binary.BigEndian.AppendUint32([]byte(nil), uint32(8+len(b))), append([]byte(typ), b...)...)
}

func TestMovieDuration(t *testing.T) {
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	u64 := func(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }
	ftyp := mp4Box("ftyp", []byte("M4A "), u32(0))
	mvhd0 := func(timescale, units uint32) []byte {
		return mp4Box("mvhd", u32(0), u32(0), u32(0), u32(timescale), u32(units), make([]byte, 80))
	}
	mvhd1 := mp4Box("mvhd", u32(1<<24), u64(0), u64(0), u32(44100), u64(30*44100+441), make([]byte, 80))
	// A preview's head: the movie box, then the start of the audio.
	preview := append(append(append([]byte(nil), ftyp...), mp4Box("moov", mvhd0(1000, 30024), mp4Box("trak"))...), mp4Box("mdat", make([]byte, 64))[:20]...)

	for _, c := range []struct {
		name string
		b    []byte
		want time.Duration
		ok   bool
	}{
		{"version 0, cut in the audio", preview, 30024 * time.Millisecond, true},
		{"version 1", append(append([]byte(nil), ftyp...), mp4Box("moov", mvhd1)...), 30*time.Second + 10*time.Millisecond, true},
		{"movie box cut short", preview[:len(ftyp)+20], 0, false},
		{"no movie box", ftyp, 0, false},
		{"unknown duration", append(append([]byte(nil), ftyp...), mp4Box("moov", mvhd0(1000, 0xffffffff))...), 0, false},
		{"no timescale", append(append([]byte(nil), ftyp...), mp4Box("moov", mvhd0(0, 30))...), 0, false},
	} {
		got, ok := MovieDuration(c.b)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: MovieDuration = %v, %v; want %v, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// Real files: the movie box first gives the length; last, nothing (the
// demuxer's own length is used then).
func TestMovieDurationFiles(t *testing.T) {
	for _, c := range []struct {
		file string
		want time.Duration
		ok   bool
	}{
		{"sine2s.m4a", 2026666666, true},
		{"sine2s-moovend.m4a", 0, false},
	} {
		b, err := os.ReadFile(filepath.Join("..", "gst", "testdata", c.file))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := MovieDuration(b[:min(len(b), 8<<10)]); got != c.want || ok != c.ok {
			t.Errorf("%s: MovieDuration = %v, %v; want %v, %v", c.file, got, ok, c.want, c.ok)
		}
	}
}
