//go:build linux || (cgo && (darwin || windows))

package gst

import (
	"errors"
	"math"
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

var testCaps = Caps{AudioConfig: []byte{0x12, 0x10}, Rate: 44100, Channels: 2}

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

// push pushes frames[from:to] as track gen, timestamped from AU 0.
func push(t *testing.T, p *Pipeline, gen uint64, frames [][]byte, from, to int) error {
	t.Helper()
	for i := from; i < to; i++ {
		b := Alloc(len(frames[i]))
		copy(b.Data, frames[i])
		if err := p.Push(gen, b, len(frames[i]), time.Duration(i)*frameDur, frameDur); err != nil {
			return err
		}
	}
	return nil
}

// waitEvent waits for an event of kind and returns the warnings before it.
func waitEvent(t *testing.T, p *Pipeline, kind EventKind, timeout time.Duration) (warnings []error) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-p.Events():
			switch ev.Kind {
			case EventError:
				t.Fatal(ev.Err)
			case EventWarning:
				warnings = append(warnings, ev.Err)
			}
			if ev.Kind == kind {
				return warnings
			}
		case <-deadline:
			t.Fatalf("no event %d within %v", kind, timeout)
		}
	}
}

func TestPlayToEOS(t *testing.T) {
	p := newTest(t, 0)
	frames := testFrames(t)
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := push(t, p, 1, frames, 0, len(frames)); err != nil {
		t.Fatal(err)
	}
	if err := p.EndOfStream(1); err != nil {
		t.Fatal(err)
	}
	// The stall watchdog must not take the preroll for a stall.
	if warns := waitEvent(t, p, EventEOS, 6*time.Second); len(warns) > 0 {
		t.Fatalf("warnings while playing: %v", warns)
	}
	total := time.Duration(len(frames)) * frameDur
	if el := time.Since(start); el < total-200*time.Millisecond {
		t.Fatalf("EOS after %v; the sink must play in real time (%v of audio)", el, total)
	}
	if gen, pos, ok := p.Position(); !ok || gen != 1 || pos < total-300*time.Millisecond {
		t.Fatalf("position at EOS %d %v %v, want track 1 at ~%v", gen, pos, ok, total)
	}
}

func TestLoadSeekClipsAndDropsStale(t *testing.T) {
	p := newTest(t, 0)
	frames := testFrames(t)
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if err := push(t, p, 1, frames, 0, 20); err != nil {
		t.Fatal(err)
	}
	// Seek to 2 s, feeding from 1.5 s like a segment boundary before the target.
	target := 2 * time.Second
	p.Load(Track{Gen: 2, Caps: testCaps, Start: target})
	b := Alloc(4)
	if err := p.Push(1, b, 4, 0, frameDur); !errors.Is(err, ErrStale) {
		t.Fatalf("stale push: %v", err)
	}
	if err := p.EndOfStream(1); !errors.Is(err, ErrStale) {
		t.Fatalf("stale EOS: %v", err)
	}
	from := int(1500 * time.Millisecond / frameDur)
	if err := push(t, p, 2, frames, from, len(frames)); err != nil {
		t.Fatal(err)
	}
	if err := p.EndOfStream(2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	gen, pos, ok := p.Position()
	if !ok || gen != 2 || pos < target || pos > target+time.Second {
		t.Fatalf("position after seek %d %v %v, want track 2 just past %v", gen, pos, ok, target)
	}
	waitEvent(t, p, EventEOS, 4*time.Second)
}

// TestLoadAsOutputPrerollsKeepsPlaying seeks right after playback starts.
// Load waits for the mixer, which holds Pipeline.mu while it waits for the
// first decoded audio, so the flush reaches the output just as that audio
// prerolls it and the pipeline sets its elements to PLAYING. The sink must
// not be left paused under a PLAYING pipeline: no position and no EOS
// until the stall watchdog rebuilds the output.
func TestLoadAsOutputPrerollsKeepsPlaying(t *testing.T) {
	frames := testFrames(t)
	for range 12 {
		p := newTest(t, 0)
		p.Load(Track{Gen: 1, Caps: testCaps})
		if err := p.Play(); err != nil {
			t.Fatal(err)
		}
		if err := push(t, p, 1, frames, 0, 20); err != nil {
			t.Fatal(err)
		}
		p.Load(Track{Gen: 2, Caps: testCaps})
		if err := push(t, p, 2, frames, 0, 5); err != nil {
			t.Fatal(err)
		}
		if err := p.EndOfStream(2); err != nil {
			t.Fatal(err)
		}
		waitEvent(t, p, EventEOS, stallAfter-time.Second)
		p.Close()
	}
}

func TestBackpressureAndLoadUnblocks(t *testing.T) {
	p := newTest(t, 500*time.Millisecond)
	frames := testFrames(t)
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- push(t, p, 1, frames, 0, len(frames)) }()
	select {
	case err := <-errc:
		t.Fatalf("push of 3 s into a 0.5 s queue while paused returned: %v", err)
	case <-time.After(700 * time.Millisecond):
	}
	if q := p.Queued(1); q < 400*time.Millisecond {
		t.Fatalf("queued %v, want ~500ms", q)
	}
	p.Load(Track{Gen: 2, Caps: testCaps})
	select {
	case err := <-errc:
		if !errors.Is(err, ErrFlushing) && !errors.Is(err, ErrStale) {
			t.Fatalf("blocked push after load: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("load did not unblock push")
	}
}

// trackDur is the test tracks' length: 1 s of the sine, after priming.
const (
	testPriming = 2112 * time.Second / 44100
	trackDur    = time.Second
)

// playOut runs the tracks queued in p to the end and returns how many
// frames the mixer wrote.
func playOut(t *testing.T, p *Pipeline, gens ...uint64) int64 {
	t.Helper()
	frames := testFrames(t)
	n := int((testPriming+trackDur)/frameDur) + 2 // a little padding past the end
	for _, g := range gens {
		go func() {
			if err := push(t, p, g, frames, 0, n); err != nil {
				t.Error(err)
			}
			_ = p.EndOfStream(g)
		}()
	}
	waitEvent(t, p, EventEOS, 10*time.Second)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mix.out
}

// TestGaplessQueueTrimsAndJoins plays two tracks back to back through the
// real decoder: priming and padding are trimmed exactly, and the second
// track starts on the frame after the first one's last.
func TestGaplessQueueTrimsAndJoins(t *testing.T) {
	p := newTest(t, 0)
	tr := Track{Caps: testCaps, Priming: testPriming, Duration: trackDur}
	tr.Gen = 1
	p.Load(tr)
	tr.Gen = 2
	if err := p.Queue(tr, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	var seen []uint64
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				if g, _, ok := p.Position(); ok && (len(seen) == 0 || seen[len(seen)-1] != g) {
					seen = append(seen, g)
				}
			}
		}
	}()
	out := playOut(t, p, 1, 2)
	close(stop)
	if want := 2 * frames(trackDur); out != want {
		t.Errorf("mixer wrote %d frames, want %d (two 1 s tracks, priming and padding trimmed)", out, want)
	}
	p.mu.Lock()
	marks := append([]mark(nil), p.mix.marks...)
	p.mu.Unlock()
	last := marks[len(marks)-1]
	if last.gen != 2 || last.frame != frames(trackDur) || last.pos != 0 {
		t.Errorf("track 2 starts at %+v, want output frame %d", last, frames(trackDur))
	}
	if len(seen) < 2 || seen[0] != 1 || seen[len(seen)-1] != 2 {
		t.Errorf("Position reported tracks %v, want 1 then 2", seen)
	}
}

func TestCrossfadeOverlapsTracks(t *testing.T) {
	p := newTest(t, 0)
	tr := Track{Caps: testCaps, Priming: testPriming, Duration: trackDur}
	tr.Gen = 1
	p.Load(tr)
	tr.Gen = 2
	fade := 400 * time.Millisecond
	if err := p.Queue(tr, fade); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	out := playOut(t, p, 1, 2)
	if want := 2*frames(trackDur) - frames(fade); out != want {
		t.Errorf("mixer wrote %d frames, want %d (two 1 s tracks overlapping by %v)", out, want, fade)
	}
}

func TestQueueAfterEndFails(t *testing.T) {
	p := newTest(t, 0)
	p.Load(Track{Gen: 1, Caps: testCaps, Priming: testPriming, Duration: trackDur})
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	playOut(t, p, 1)
	if err := p.Queue(Track{Gen: 2, Caps: testCaps}, 0); !errors.Is(err, ErrEnded) {
		t.Fatalf("queue after the end: %v, want ErrEnded", err)
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
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- push(t, p, 1, frames, 0, len(frames)) }()
	time.Sleep(500 * time.Millisecond) // the queue is full; push blocks
	p.Close()
	if err := <-errc; err == nil {
		t.Fatal("push into a closed pipeline succeeded")
	}
	b := Alloc(4)
	if err := p.Push(1, b, 4, 0, frameDur); !errors.Is(err, ErrClosed) {
		t.Fatalf("push after close: %v", err)
	}
	if _, _, ok := p.Position(); ok {
		t.Fatal("position after close")
	}
}

func TestSetDeviceKeepsPlaying(t *testing.T) {
	p := newTest(t, 0)
	frames := testFrames(t)
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = push(t, p, 1, frames, 0, len(frames)) }()
	time.Sleep(800 * time.Millisecond)
	_, before, _ := p.Position()
	if err := p.SetDevice("elsewhere"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	gen, after, ok := p.Position()
	if !ok || gen != 1 || after <= before {
		t.Fatalf("after rebuilding the output: track %d at %v (%v), was at %v", gen, after, ok, before)
	}
}

func TestEqualizerAndVolumeReachTheLiveOutput(t *testing.T) {
	if !Have("equalizer-10bands") {
		t.Skip("equalizer-10bands not installed")
	}
	p := newTest(t, 2*time.Second)
	bands := [EQBands]float64{6, 4, 2, 0, 0, 0, -1, -2, -3, -12}
	p.SetEqualizer(-6, bands)
	p.SetVolume(0.125)
	got, ok := p.Output()
	if !ok || !got.HasEQ {
		t.Fatalf("Output() = %+v, %v", got, ok)
	}
	if got.Bands != bands || math.Abs(got.Pregain+6) > 1e-4 || got.Volume != 0.125 {
		t.Errorf("live output %+v, want bands %v, pregain −6 dB, volume 0.125", got, bands)
	}
	// Changes while playing apply to the same elements.
	frames := testFrames(t)
	p.Load(Track{Gen: 1, Caps: testCaps})
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = push(t, p, 1, frames, 0, len(frames)) }()
	p.SetEqualizer(0, [EQBands]float64{})
	if got, _ := p.Output(); got.Bands != ([EQBands]float64{}) || got.Pregain != 0 {
		t.Errorf("after flat: %+v", got)
	}
}
