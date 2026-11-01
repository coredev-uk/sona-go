package broadcast

import (
	"slices"
	"testing"
	"time"
)

func samples(n, rate int) time.Duration { return time.Duration(n) * time.Second / time.Duration(rate) }

func TestParseFrame(t *testing.T) {
	for _, c := range []struct {
		name string
		h    []byte
		size int
		dur  time.Duration
	}{
		{"ADTS 44.1 kHz", adtsFrame(4, 200, 1, 0), 200, samples(1024, 44100)},
		{"ADTS 48 kHz, two blocks", adtsFrame(3, 300, 2, 0), 300, samples(2048, 48000)},
		{"ADTS long frame", adtsFrame(11, 3000, 1, 0), 3000, samples(1024, 8000)},
		{"MPEG-1 layer III", mpegFrame(0xfb, 0x90, 8, 0), 417, samples(1152, 44100)},
		{"MPEG-1 layer III padded", mpegFrame(0xfb, 0x92, 8, 0), 418, samples(1152, 44100)},
		{"MPEG-1 layer III no CRC bit", mpegFrame(0xfa, 0x90, 8, 0), 417, samples(1152, 44100)},
		{"MPEG-1 layer II", mpegFrame(0xfd, 0xa4, 8, 0), 576, 24 * time.Millisecond},
		{"MPEG-1 layer I", mpegFrame(0xff, 0xc8, 8, 0), 576, 12 * time.Millisecond},
		{"MPEG-1 layer I padded", mpegFrame(0xff, 0xca, 8, 0), 580, 12 * time.Millisecond},
		{"MPEG-2 layer III", mpegFrame(0xf3, 0x80, 8, 0), 208, samples(576, 22050)},
		{"MPEG-2 layer II", mpegFrame(0xf5, 0x80, 8, 0), 417, samples(1152, 22050)},
		{"MPEG-2 layer I", mpegFrame(0xf7, 0x10, 8, 0), 68, samples(384, 22050)},
		{"MPEG-2.5 layer III", mpegFrame(0xe3, 0x40, 8, 0), 208, samples(576, 11025)},
	} {
		size, dur, ok := parseFrame(c.h)
		if !ok || size != c.size || dur != c.dur {
			t.Errorf("%s: %d %v %v, want %d %v", c.name, size, dur, ok, c.size, c.dur)
		}
	}
}

func TestParseFrameRejects(t *testing.T) {
	short := adtsFrame(4, 200, 1, 0)
	short[3], short[4], short[5] = 0x80, 0, 6<<5 // 6 bytes
	for name, h := range map[string][]byte{
		"zeros":               make([]byte, 7),
		"ID3":                 []byte("ID3\x04\x00\x00\x00"),
		"no sync":             {0xff, 0x00, 0, 0, 0, 0, 0},
		"ADTS rate index 13":  adtsFrame(13, 200, 1, 0),
		"ADTS shorter than 7": short,
		"MPEG reserved ver":   mpegFrame(0xeb, 0x90, 7, 0),
		"MPEG reserved layer": mpegFrame(0xf9, 0x90, 7, 0),
		"MPEG free bitrate":   mpegFrame(0xfb, 0x00, 7, 0),
		"MPEG bad bitrate":    mpegFrame(0xfb, 0xf0, 7, 0),
		"MPEG reserved rate":  mpegFrame(0xfb, 0x9c, 7, 0),
	} {
		if size, dur, ok := parseFrame(h); ok {
			t.Errorf("%s: parsed as %d bytes, %v", name, size, dur)
		}
	}
}

func TestClock(t *testing.T) {
	var stream []byte
	var want time.Duration
	add := func(b []byte, d time.Duration) {
		stream = append(stream, b...)
		want += d
	}
	add([]byte{0, 0, 0xff, 0x00, 0}, 0) // junk before the first frame
	add(adtsFrame(4, 200, 1, 0xff), samples(1024, 44100))
	add(mp3Frames(1, 0xff), mp3Dur)
	add([]byte{0x47, 0xff, 0xe0, 0}, 0) // junk between frames, with a false sync
	add(adtsFrame(3, 300, 2, 0xf1), samples(2048, 48000))
	add(mpegFrame(0xe3, 0x40, 208, 0xff), samples(576, 11025))
	add(mpegFrame(0xfd, 0xa4, 576, 0), 24*time.Millisecond)

	for _, chunk := range []int{1, 2, 3, 6, 7, 8, 13, 199, 417, len(stream)} {
		var c clock
		for b := stream; len(b) > 0; {
			n := min(chunk, len(b))
			c.write(b[:n])
			b = b[n:]
		}
		if c.d != want {
			t.Errorf("chunks of %d: %v, want %v", chunk, c.d, want)
		}
	}
}

func TestClockPartialFrames(t *testing.T) {
	frame := adtsFrame(4, 200, 1, 0)
	two := slices.Concat(frame, frame)
	var c clock
	c.write(two[:6]) // not yet a whole header
	if c.d != 0 {
		t.Fatalf("partial header: %v", c.d)
	}
	c.write(two[6:7]) // the header completes: the frame counts
	if c.d != aacDur {
		t.Fatalf("header: %v", c.d)
	}
	c.write(two[7:205]) // the rest of the frame and part of the next header
	if c.d != aacDur {
		t.Fatalf("partial second header: %v", c.d)
	}
	c.write(two[205:])
	if c.d != 2*aacDur {
		t.Fatalf("second frame: %v", c.d)
	}
	c.write(nil)
	if c.d != 2*aacDur {
		t.Fatalf("empty write: %v", c.d)
	}
}
