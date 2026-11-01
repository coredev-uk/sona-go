package broadcast

import (
	"errors"
	"fmt"
)

// MPEG transport stream (ISO/IEC 13818-1) segments, as some broadcasters'
// HLS streams carry them: the demuxer finds the program's audio stream and
// its timed ID3 stream (HLS timed metadata) through the PAT and PMT, and
// hands on the audio elementary stream (ADTS AAC or MPEG audio frames)
// and each ID3 tag. Each HLS segment starts with a PAT and a PMT and
// whole PES packets, so the demuxer flushes at the end of every segment.

const tsPacket = 188

// Stream types (PMT) the demuxer reads.
const (
	streamMPEG1Audio = 0x03
	streamMPEG2Audio = 0x04
	streamADTS       = 0x0f
	streamID3        = 0x15 // metadata in PES packets
)

var errNoAudio = errors.New("broadcast: transport stream without an ADTS or MPEG audio stream")

// isTS reports whether a segment is an MPEG transport stream.
func isTS(b []byte) bool {
	return len(b) >= tsPacket && b[0] == 0x47 && (len(b) == tsPacket || len(b) > tsPacket && b[tsPacket] == 0x47)
}

// tsDemuxer demuxes transport stream segments. The PMT and stream PIDs
// carry over from one segment to the next.
type tsDemuxer struct {
	pmtPID   int // -1 until the PAT names it
	audioPID int // -1 until the PMT names it
	id3PID   int
	audio    []byte // the PES packet being collected on audioPID
	id3      []byte // … on id3PID
}

func newTSDemuxer() *tsDemuxer { return &tsDemuxer{pmtPID: -1, audioPID: -1, id3PID: -1} }

// segment demuxes one segment: audio gets the audio elementary stream
// (each PES packet's payload), meta each ID3 tag.
func (d *tsDemuxer) segment(b []byte, audio func([]byte) error, meta func([]byte)) error {
	for off := 0; off+tsPacket <= len(b); off += tsPacket {
		p := b[off : off+tsPacket]
		if p[0] != 0x47 {
			return fmt.Errorf("broadcast: transport stream lost sync at byte %d", off)
		}
		pusi := p[1]&0x40 != 0
		pid := int(p[1]&0x1f)<<8 | int(p[2])
		payload := p[4:]
		switch p[3] >> 4 & 3 { // adaptation field control
		case 0, 2: // reserved, or no payload
			continue
		case 3:
			if int(p[4]) >= len(payload) {
				continue
			}
			payload = payload[1+int(p[4]):]
		}
		var err error
		switch {
		case pid == 0:
			d.pat(payload, pusi)
		case pid == d.pmtPID:
			err = d.pmt(payload, pusi)
		case pid == d.audioPID:
			if pusi {
				err = d.flushAudio(audio)
			}
			d.audio = append(d.audio, payload...)
		case pid == d.id3PID:
			if pusi {
				d.flushID3(meta)
			}
			d.id3 = append(d.id3, payload...)
		}
		if err != nil {
			return err
		}
	}
	if d.audioPID < 0 && d.pmtPID >= 0 {
		return errNoAudio
	}
	d.flushID3(meta)
	return d.flushAudio(audio)
}

// section returns the PSI section at the start of a packet's payload
// (after its pointer field), or nil.
func section(payload []byte, pusi bool) []byte {
	if !pusi || len(payload) < 1 || 1+int(payload[0]) >= len(payload) {
		return nil
	}
	s := payload[1+int(payload[0]):]
	if len(s) < 3 {
		return nil
	}
	n := int(s[1]&0x0f)<<8 | int(s[2])
	if 3+n > len(s) || n < 9 {
		return nil
	}
	return s[:3+n-4] // without the CRC
}

func (d *tsDemuxer) pat(payload []byte, pusi bool) {
	s := section(payload, pusi)
	if s == nil || s[0] != 0x00 {
		return
	}
	for e := s[8:]; len(e) >= 4; e = e[4:] {
		if program := int(e[0])<<8 | int(e[1]); program != 0 {
			d.pmtPID = int(e[2]&0x1f)<<8 | int(e[3])
			return
		}
	}
}

func (d *tsDemuxer) pmt(payload []byte, pusi bool) error {
	s := section(payload, pusi)
	if s == nil || s[0] != 0x02 || len(s) < 12 {
		return nil
	}
	info := int(s[10]&0x0f)<<8 | int(s[11])
	if 12+info > len(s) {
		return nil
	}
	audio, id3, other := -1, -1, -1
	for e := s[12+info:]; len(e) >= 5; {
		typ, pid := e[0], int(e[1]&0x1f)<<8|int(e[2])
		switch typ {
		case streamADTS, streamMPEG1Audio, streamMPEG2Audio:
			if audio < 0 {
				audio = pid
			}
		case streamID3:
			if id3 < 0 {
				id3 = pid
			}
		default:
			other = int(typ)
		}
		n := 5 + (int(e[3]&0x0f)<<8 | int(e[4]))
		if n > len(e) {
			break
		}
		e = e[n:]
	}
	if audio < 0 {
		if other >= 0 {
			return fmt.Errorf("broadcast: transport stream audio of stream type %#x is not supported", other)
		}
		return errNoAudio
	}
	d.audioPID, d.id3PID = audio, id3
	return nil
}

func (d *tsDemuxer) flushAudio(audio func([]byte) error) error {
	pes := d.audio
	d.audio = d.audio[:0]
	if data := pesPayload(pes); len(data) > 0 {
		return audio(data)
	}
	return nil
}

func (d *tsDemuxer) flushID3(meta func([]byte)) {
	pes := d.id3
	d.id3 = d.id3[:0]
	if data := pesPayload(pes); len(data) > 0 {
		meta(data)
	}
}

// pesPayload returns a PES packet's payload: what follows its header.
func pesPayload(p []byte) []byte {
	if len(p) < 9 || p[0] != 0 || p[1] != 0 || p[2] != 1 {
		return nil
	}
	n := 9 + int(p[8])
	if n > len(p) {
		return nil
	}
	return p[n:]
}
