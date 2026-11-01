package broadcast

import (
	"bytes"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/id3/id3test"
)

// Synthetic streams for the tests: ADTS and MPEG audio frames, MPEG
// transport stream packets and ID3 tags.

// aacDur is the duration of one ADTS frame at 44.1 kHz (sample rate index 4).
const aacDur = 1024 * time.Second / 44100

// adtsFrame returns an ADTS AAC-LC stereo frame of n bytes (header without
// CRC) at sample rate index sfi holding blocks raw data blocks, its body
// filled with fill.
func adtsFrame(sfi, n, blocks int, fill byte) []byte {
	f := bytes.Repeat([]byte{fill}, n)
	f[0], f[1] = 0xff, 0xf1
	f[2] = byte(1<<6 | sfi<<2)
	f[3] = byte(2<<6 | n>>11&0x03)
	f[4] = byte(n >> 3)
	f[5] = byte(n&0x07<<5 | 0x1f)
	f[6] = byte(0xfc | (blocks-1)&0x03)
	return f
}

// aacFrames returns n 44.1 kHz ADTS frames of 200 bytes filled with fill.
func aacFrames(n int, fill byte) []byte {
	var b []byte
	for range n {
		b = append(b, adtsFrame(4, 200, 1, fill)...)
	}
	return b
}

// mpegFrame returns an MPEG audio frame of n bytes with header bytes h1
// and h2 (sync, version, layer; bitrate, rate, padding).
func mpegFrame(h1, h2 byte, n int, fill byte) []byte {
	f := bytes.Repeat([]byte{fill}, n)
	f[0], f[1], f[2], f[3] = 0xff, h1, h2, 0x44
	return f
}

// MPEG-1 layer III, 128 kbps, 44.1 kHz, no padding: 417 bytes, 1152 samples.
const (
	mp3Size = 417
	mp3Dur  = 1152 * time.Second / 44100
)

// mp3Frames returns n MPEG-1 layer III frames (128 kbps, 44.1 kHz).
func mp3Frames(n int, fill byte) []byte {
	var b []byte
	for range n {
		b = append(b, mpegFrame(0xfb, 0x90, mp3Size, fill)...)
	}
	return b
}

// Transport stream PIDs of the synthetic segments.
const (
	pidPMT   = 0x1000
	pidAudio = 0x100
	pidID3   = 0x101
	pidVideo = 0x102
)

// tsPkt returns a 188-byte transport stream packet. af is the adaptation
// field after its length byte (flags, …); the packet gets one, stuffed,
// when af is set or payload does not fill the packet.
func tsPkt(pid int, pusi bool, af, payload []byte) []byte {
	h1 := byte(pid>>8) & 0x1f
	if pusi {
		h1 |= 0x40
	}
	p := []byte{0x47, h1, byte(pid), 0x10}
	if af != nil || len(payload) < 184 {
		p[3] = 0x30
		l := 183 - len(payload)
		if l > 0 && len(af) == 0 {
			af = []byte{0}
		}
		p = append(append(p, byte(l)), af...)
		for len(p) < 5+l {
			p = append(p, 0xff)
		}
	}
	return append(p, payload...)
}

// tsAFOnly returns a packet of pid with an adaptation field (a PCR) and no
// payload.
func tsAFOnly(pid int) []byte {
	p := tsPkt(pid, false, []byte{0x10, 1, 2, 3, 4, 5, 6}, bytes.Repeat([]byte{0xee}, 176))
	p[3] = 0x20 // adaptation field only: the bytes after it are stuffing
	p[4] = 183
	return p
}

// tsPSI returns a packet carrying a PSI section (pointer field 0).
func tsPSI(pid int, section []byte) []byte {
	payload := append([]byte{0}, section...)
	payload = append(payload, bytes.Repeat([]byte{0xff}, 184-len(payload))...)
	return tsPkt(pid, true, nil, payload)
}

// patSection returns a PAT naming a network PID (program 0) and program
// 1's PMT at pidPMT.
func patSection() []byte {
	s := []byte{0x00, 0xb0, 0, 0, 1, 0xc1, 0, 0,
		0, 0, 0xe0, 0x10, // program 0: network PID
		0, 1, 0xe0 | pidPMT>>8, pidPMT & 0xff,
		0, 0, 0, 0} // CRC (not checked)
	s[2] = byte(len(s) - 3)
	return s
}

type esEntry struct {
	typ  byte
	pid  int
	info []byte
}

// pmtSection returns program 1's PMT, with a program descriptor and the
// elementary streams es.
func pmtSection(es ...esEntry) []byte {
	info := []byte{0x05, 4, 'C', 'U', 'E', 'I'}
	s := []byte{0x02, 0xb0, 0, 0, 1, 0xc1, 0, 0, 0xe0 | pidAudio>>8, pidAudio & 0xff, 0xf0, byte(len(info))}
	s = append(s, info...)
	for _, e := range es {
		s = append(s, e.typ, 0xe0|byte(e.pid>>8), byte(e.pid), 0xf0, byte(len(e.info)))
		s = append(s, e.info...)
	}
	s = append(s, 0, 0, 0, 0) // CRC
	s[2] = byte(len(s) - 3)
	return s
}

// pesPacket returns a PES packet of stream sid with a PTS, holding data.
func pesPacket(sid byte, data []byte) []byte {
	n := 3 + 5 + len(data)
	if n > 0xffff {
		n = 0
	}
	p := []byte{0, 0, 1, sid, byte(n >> 8), byte(n), 0x80, 0x80, 5, 0x21, 0, 1, 0, 1}
	return append(p, data...)
}

// tsPES splits a PES packet into transport stream packets of pid, the
// first with a PCR when pcr is set.
func tsPES(pid int, pes []byte, pcr bool) []byte {
	var out []byte
	for first := true; first || len(pes) > 0; first = false {
		var af []byte
		room := 184
		if first && pcr {
			af = []byte{0x10, 0, 0, 0, 0, 0x7e, 0}
			room = 184 - 1 - len(af)
		}
		n := min(room, len(pes))
		out = append(out, tsPkt(pid, first, af, pes[:n])...)
		pes = pes[n:]
	}
	return out
}

// tsSegment returns an HLS segment: PAT, PMT (ADTS audio and timed ID3),
// the ID3 tag (if any) and one audio PES packet holding audio.
func tsSegment(audio, tag []byte) []byte {
	b := tsPSI(0, patSection())
	b = append(b, tsPSI(pidPMT, pmtSection(esEntry{streamADTS, pidAudio, nil}, esEntry{streamID3, pidID3, []byte{0x26, 2, 0xff, 0xff}}))...)
	if tag != nil {
		b = append(b, tsPES(pidID3, pesPacket(0xbd, tag), false)...)
	}
	return append(b, tsPES(pidAudio, pesPacket(0xc0, audio), true)...)
}

// onAirTag returns an ID3v2.4 tag naming title and artist.
func onAirTag(title, artist string) []byte {
	return id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text(title)), id3test.Frame(4, "TPE1", id3test.Text(artist)))
}

// timestampTag returns the ID3 tag packed audio segments start with: a
// PRIV frame with the transport stream timestamp, no title.
func timestampTag() []byte {
	priv := append([]byte("com.apple.streaming.transportStreamTimestamp\x00"), 0, 0, 0, 0, 0, 0, 0x1c, 0x20)
	return id3test.Tag(4, id3test.Frame(4, "PRIV", priv))
}

// icyBlock returns an ICY metadata block (length byte, NUL-padded text).
func icyBlock(s string) []byte {
	n := (len(s) + 15) / 16
	b := append([]byte{byte(n)}, s...)
	return append(b, make([]byte, n*16-len(s))...)
}
