package broadcast

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type demuxed struct {
	audio [][]byte
	meta  [][]byte
}

func (d *demuxed) segment(t *testing.T, dm *tsDemuxer, b []byte) error {
	t.Helper()
	return dm.segment(b, func(a []byte) error {
		d.audio = append(d.audio, bytes.Clone(a))
		return nil
	}, func(m []byte) {
		d.meta = append(d.meta, bytes.Clone(m))
	})
}

func TestTSDemux(t *testing.T) {
	first := aacFrames(5, 0x11) // several packets
	second := aacFrames(1, 0x22)
	tag := onAirTag("Song", "Artist")
	var seg []byte
	seg = append(seg, tsPSI(0, patSection())...)
	seg = append(seg, tsPkt(0x1fff, false, nil, bytes.Repeat([]byte{0xff}, 184))...) // null packet
	seg = append(seg, tsPSI(pidPMT, pmtSection(
		esEntry{0x1b, pidVideo, nil}, // H.264: ignored
		esEntry{streamADTS, pidAudio, []byte{0x0a, 4, 'e', 'n', 'g', 0}},
		esEntry{streamID3, pidID3, nil},
	))...)
	audio := tsPES(pidAudio, pesPacket(0xc0, first), true)
	// An adaptation-field-only packet and a video packet in the middle of
	// the audio PES packet.
	seg = append(seg, audio[:2*188]...)
	seg = append(seg, tsAFOnly(pidAudio)...)
	seg = append(seg, tsPES(pidVideo, pesPacket(0xe0, []byte("video")), false)...)
	seg = append(seg, audio[2*188:]...)
	seg = append(seg, tsPES(pidID3, pesPacket(0xbd, tag), false)...)
	seg = append(seg, tsPES(pidAudio, pesPacket(0xc0, second), false)...)

	dm := newTSDemuxer()
	var got demuxed
	if err := got.segment(t, dm, seg); err != nil {
		t.Fatal(err)
	}
	if len(got.audio) != 2 || !bytes.Equal(got.audio[0], first) || !bytes.Equal(got.audio[1], second) {
		t.Fatalf("audio: %d PES packets", len(got.audio))
	}
	if len(got.meta) != 1 || !bytes.Equal(got.meta[0], tag) {
		t.Fatalf("meta: %q", got.meta)
	}

	// The next segment's PIDs carry over, without a PAT or PMT; a PES
	// packet split across segments is not: each segment ends its own.
	third := aacFrames(3, 0x33)
	got = demuxed{}
	if err := got.segment(t, dm, tsPES(pidAudio, pesPacket(0xc0, third), false)); err != nil {
		t.Fatal(err)
	}
	if len(got.audio) != 1 || !bytes.Equal(got.audio[0], third) || len(got.meta) != 0 {
		t.Fatalf("second segment: %d audio, %d meta", len(got.audio), len(got.meta))
	}
}

func TestTSDemuxMPEGAudioWithoutID3(t *testing.T) {
	for _, typ := range []byte{streamMPEG1Audio, streamMPEG2Audio} {
		frames := mp3Frames(2, 0x55)
		seg := append(tsPSI(0, patSection()), tsPSI(pidPMT, pmtSection(esEntry{typ, pidAudio, nil}))...)
		seg = append(seg, tsPES(pidAudio, pesPacket(0xc0, frames), false)...)
		// A timed ID3 PES on a PID the PMT does not name is ignored.
		seg = append(seg, tsPES(pidID3, pesPacket(0xbd, onAirTag("x", "y")), false)...)
		var got demuxed
		if err := got.segment(t, newTSDemuxer(), seg); err != nil {
			t.Fatalf("type %#x: %v", typ, err)
		}
		if len(got.audio) != 1 || !bytes.Equal(got.audio[0], frames) || len(got.meta) != 0 {
			t.Fatalf("type %#x: %d audio, %d meta", typ, len(got.audio), len(got.meta))
		}
	}
}

func TestTSDemuxSkipsBrokenPES(t *testing.T) {
	frames := aacFrames(1, 0x44)
	broken := pesPacket(0xc0, aacFrames(1, 0x66))
	broken[2] = 0x02 // no start code
	truncated := pesPacket(0xc0, nil)[:8]
	seg := tsSegment(frames, nil)
	seg = append(tsPES(pidAudio, broken, false), seg...) // before the PAT: no PID known yet
	seg = append(seg, tsPES(pidAudio, broken, false)...)
	seg = append(seg, tsPES(pidAudio, truncated, false)...)
	var got demuxed
	if err := got.segment(t, newTSDemuxer(), seg); err != nil {
		t.Fatal(err)
	}
	if len(got.audio) != 1 || !bytes.Equal(got.audio[0], frames) {
		t.Fatalf("audio: %d PES packets", len(got.audio))
	}
}

func TestTSDemuxErrors(t *testing.T) {
	pat := tsPSI(0, patSection())[:188:188] // appended to below: never shared
	audioErr := errors.New("sink full")
	lostSync := tsSegment(aacFrames(1, 1), nil)
	lostSync[2*188] = 0x48
	for _, c := range []struct {
		name string
		seg  []byte
		want func(error) bool
	}{
		{"video only", append(pat, tsPSI(pidPMT, pmtSection(esEntry{0x1b, pidVideo, nil}))...), func(err error) bool {
			return err != nil && strings.Contains(err.Error(), "0x1b")
		}},
		{"no streams", append(pat, tsPSI(pidPMT, pmtSection())...), func(err error) bool { return errors.Is(err, errNoAudio) }},
		{"PAT without PMT", pat, func(err error) bool { return errors.Is(err, errNoAudio) }},
		{"lost sync", lostSync, func(err error) bool { return err != nil && strings.Contains(err.Error(), "sync at byte 376") }},
		{"audio error", tsSegment(aacFrames(1, 1), nil), func(err error) bool { return errors.Is(err, audioErr) }},
	} {
		err := newTSDemuxer().segment(c.seg, func([]byte) error { return audioErr }, func([]byte) {})
		if !c.want(err) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// No PAT at all: nothing to say yet.
	if err := newTSDemuxer().segment(tsPES(pidAudio, pesPacket(0xc0, aacFrames(1, 1)), false), nil, nil); err != nil {
		t.Errorf("no PAT: %v", err)
	}
}

func TestIsTS(t *testing.T) {
	two := append(tsPSI(0, patSection()), tsPSI(0, patSection())...)
	for _, c := range []struct {
		name string
		b    []byte
		want bool
	}{
		{"one packet", two[:188], true},
		{"two packets", two, true},
		{"one and a bit", two[:200], true},
		{"short", two[:187], false},
		{"second packet out of sync", append(two[:188:188], 0x00), false},
		{"ADTS", aacFrames(2, 0x47), false},
		{"ID3", append(onAirTag("a", "b"), aacFrames(2, 0)...), false},
	} {
		if got := isTS(c.b); got != c.want {
			t.Errorf("%s: isTS = %v", c.name, got)
		}
	}
}
