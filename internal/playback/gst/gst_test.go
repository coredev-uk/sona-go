package gst

import (
	"errors"
	"os"
	"testing"
	"time"
)

// sine3s.adts: 3 s of a 440 Hz sine, AAC-LC 44.1 kHz stereo, generated with
// audiotestsrc ! avenc_aac.
func testFrames(t *testing.T) [][]byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sine3s.adts")
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for len(b) >= 7 {
		n := int(b[3]&3)<<11 | int(b[4])<<3 | int(b[5])>>5
		hdr := 7
		if b[1]&1 == 0 {
			hdr = 9
		}
		frames = append(frames, b[hdr:n])
		b = b[n:]
	}
	return frames
}

var testCaps = &Caps{AudioConfig: []byte{0x12, 0x10}, Rate: 44100, Channels: 2}

const frameDur = time.Second * 1024 / 44100

func newTest(t *testing.T, maxQueue time.Duration) *Pipeline {
	t.Helper()
	if !Have("avdec_aac") && !Have("fdkaacdec") && !Have("faad") {
		t.Skip("no AAC decoder installed")
	}
	p, err := New(Options{Sink: "fakesink", MaxQueue: maxQueue})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func push(t *testing.T, p *Pipeline, gen uint64, frames [][]byte, from int) error {
	t.Helper()
	for i := from; i < len(frames); i++ {
		b := Alloc(len(frames[i]))
		copy(b.Data, frames[i])
		if err := p.Push(gen, b, len(frames[i]), time.Duration(i)*frameDur, frameDur); err != nil {
			return err
		}
	}
	return nil
}

func waitEvent(t *testing.T, p *Pipeline, kind EventKind, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-p.Events():
			if ev.Kind == EventError {
				t.Fatal(ev.Err)
			}
			if ev.Kind == kind {
				return
			}
		case <-deadline:
			t.Fatalf("no event %d within %v", kind, timeout)
		}
	}
}

func TestPlayToEOS(t *testing.T) {
	p := newTest(t, 0)
	frames := testFrames(t)
	p.Flush(1, 0, testCaps)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	errc := make(chan error, 1)
	go func() { errc <- push(t, p, 1, frames, 0) }()
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if err := p.EndOfStream(1); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, p, EventEOS, 6*time.Second)
	total := time.Duration(len(frames)) * frameDur
	if el := time.Since(start); el < total-200*time.Millisecond {
		t.Fatalf("EOS after %v; the sink must play in real time (%v of audio)", el, total)
	}
	if pos, ok := p.Position(); !ok || pos < total-300*time.Millisecond {
		t.Fatalf("position at EOS %v %v, want ~%v", pos, ok, total)
	}
}

func TestFlushSeekClipsAndDropsStale(t *testing.T) {
	p := newTest(t, 0)
	frames := testFrames(t)
	p.Flush(1, 0, testCaps)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if err := push(t, p, 1, frames[:20], 0); err != nil {
		t.Fatal(err)
	}
	// Seek to 2 s, feeding from 1.5 s like a segment boundary before the target.
	target := 2 * time.Second
	p.Flush(2, target, nil)
	b := Alloc(4)
	if err := p.Push(1, b, 4, 0, frameDur); !errors.Is(err, ErrStale) {
		t.Fatalf("stale push: %v", err)
	}
	if err := p.EndOfStream(1); !errors.Is(err, ErrStale) {
		t.Fatalf("stale EOS: %v", err)
	}
	from := int(1500 * time.Millisecond / frameDur)
	if err := push(t, p, 2, frames, from); err != nil {
		t.Fatal(err)
	}
	if err := p.EndOfStream(2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	pos, ok := p.Position()
	if !ok || pos < target || pos > target+time.Second {
		t.Fatalf("position after seek %v %v, want just past %v", pos, ok, target)
	}
	waitEvent(t, p, EventEOS, 4*time.Second)
}

func TestBackpressureAndFlushUnblocks(t *testing.T) {
	p := newTest(t, 500*time.Millisecond)
	frames := testFrames(t)
	p.Flush(1, 0, testCaps)
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- push(t, p, 1, frames, 0) }()
	select {
	case err := <-errc:
		t.Fatalf("push of 3 s into a 0.5 s queue while paused returned: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if q := p.Queued(); q < 400*time.Millisecond {
		t.Fatalf("queued %v, want ~500ms", q)
	}
	p.Flush(2, 0, nil)
	select {
	case err := <-errc:
		if !errors.Is(err, ErrFlushing) && !errors.Is(err, ErrStale) {
			t.Fatalf("blocked push after flush: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not unblock push")
	}
}

func TestProbe(t *testing.T) {
	info, err := Probe()
	if err != nil {
		t.Skip("no usable audio output:", err)
	}
	t.Logf("GStreamer %s, decoder %s, sink %s", info.Version, info.Decoder, info.Sink)
	if info.Version == "" || info.Decoder == "" || info.Sink == "" {
		t.Fatalf("%+v", info)
	}
}

func TestCloseWhilePushBlocked(t *testing.T) {
	p, err := New(Options{Sink: "fakesink", MaxQueue: 200 * time.Millisecond})
	if err != nil {
		t.Skip("no pipeline:", err)
	}
	frames := testFrames(t)
	p.Flush(1, 0, testCaps)
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- push(t, p, 1, frames, 0) }()
	time.Sleep(300 * time.Millisecond) // the queue is full; push blocks
	p.Close()
	if err := <-errc; err == nil {
		t.Fatal("push into a closed pipeline succeeded")
	}
	b := Alloc(4)
	if err := p.Push(1, b, 4, 0, frameDur); !errors.Is(err, ErrClosed) {
		t.Fatalf("push after close: %v", err)
	}
	if _, ok := p.Position(); ok {
		t.Fatal("position after close")
	}
}
