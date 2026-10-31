//go:build linux

package cdmhost

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/cdm/cdmtest"
)

// The test binary doubles as the child process, and as the probe and
// misbehaving children of sandbox_test.go and frames_test.go.
func TestMain(m *testing.M) {
	testChildModes()
	ChildMain()
	os.Exit(m.Run())
}

func startHost(t *testing.T) *Host {
	t.Helper()
	lib := cdmtest.Library(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := Start(ctx, Options{CDMPath: lib, Sink: "fakesink"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestProbe(t *testing.T) {
	lib := cdmtest.Library(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := Probe(ctx, Options{CDMPath: lib, Sink: "fakesink"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", info)
	if info.PID == 0 || info.CDMPath != lib || info.CDMVersion == "" || info.CDMInterface == 0 || info.GStreamer == "" || info.Decoder == "" || info.Sink != "fakesink" {
		t.Fatalf("incomplete probe: %+v", info)
	}
}

func TestProbeMissingCDM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := Probe(ctx, Options{CDMPath: "/nonexistent/libwidevinecdm.so", Sink: "fakesink"})
	if !errors.Is(err, ErrCDMLoad) {
		t.Fatalf("probe with a missing CDM: %v, want ErrCDMLoad", err)
	}
	if info.GStreamer == "" || info.PID == 0 {
		t.Fatalf("probe should still report the audio stack: %+v", info)
	}
}

// A file that is not a CDM fails to load: ErrCDMLoad, which the engine
// counts towards rolling back a downloaded version.
func TestProbeBrokenCDM(t *testing.T) {
	lib := t.TempDir() + "/libwidevinecdm.so"
	if err := os.WriteFile(lib, []byte("not a shared library"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := Probe(ctx, Options{CDMPath: lib, Sink: "fakesink"})
	if errors.Is(err, ErrSandbox) {
		t.Skip("no seccomp filter here, so the CDM is never loaded:", err)
	}
	if !errors.Is(err, ErrCDMLoad) || info.CDMPath != lib || info.CDMInterface != 0 {
		t.Fatalf("probe with a broken CDM: %v %+v, want ErrCDMLoad", err, info)
	}
}

func TestSessionChallenge(t *testing.T) {
	h := startHost(t)
	ctx := context.Background()
	kid := bytes.Repeat([]byte{7}, 16)
	sid, challenge, err := h.CreateSession(ctx, kid)
	if err != nil || sid == "" || len(challenge) < 100 {
		t.Fatalf("session %q, %d-byte challenge, %v", sid, len(challenge), err)
	}
	if err := h.UpdateSession(ctx, sid, []byte("garbage")); err == nil {
		t.Fatal("garbage licence accepted")
	}
	if err := h.CloseSession(ctx, sid); err != nil {
		t.Fatal(err)
	}
}

// A clear (unencrypted) fMP4 AAC track built from the gst test sine.

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func be16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

func mkbox(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	return bytes.Join([][]byte{be32(uint32(8 + len(body))), []byte(typ), body}, nil)
}

func mkfull(typ string, version uint8, flags uint32, parts ...[]byte) []byte {
	return mkbox(typ, append([][]byte{be32(uint32(version)<<24 | flags)}, parts...)...)
}

func clearInit() []byte {
	asc := []byte{0x12, 0x10}
	d := func(tag byte, body []byte) []byte { return append([]byte{tag, byte(len(body))}, body...) }
	dcd := d(0x04, append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, d(0x05, asc)...))
	esds := mkfull("esds", 0, 0, d(0x03, append([]byte{0, 0, 0}, dcd...)))
	entry := mkbox("mp4a", make([]byte, 6), be16(1), make([]byte, 8), be16(2), be16(16), be32(0), be32(44100<<16), esds)
	stsd := mkfull("stsd", 0, 0, be32(1), entry)
	mdhd := mkfull("mdhd", 0, 0, be32(0), be32(0), be32(44100), be32(0), be32(0))
	tkhd := mkfull("tkhd", 0, 7, be32(0), be32(0), be32(1), make([]byte, 68))
	trak := mkbox("trak", tkhd, mkbox("mdia", mdhd, mkbox("minf", mkbox("stbl", stsd))))
	return bytes.Join([][]byte{mkbox("ftyp", []byte("iso8"), be32(0)), mkbox("moov", trak, mkbox("mvex", mkfull("trex", 0, 0, be32(1), be32(1), be32(1024), be32(0), be32(0))))}, nil)
}

func fragment(base uint64, frames [][]byte) []byte {
	var sizes, data []byte
	for _, f := range frames {
		sizes = append(sizes, be32(uint32(len(f)))...)
		data = append(data, f...)
	}
	build := func(off uint32) []byte {
		traf := mkbox("traf", mkfull("tfhd", 0, 0x020008, be32(1), be32(1024)),
			mkfull("tfdt", 1, 0, binary.BigEndian.AppendUint64(nil, base)),
			mkfull("trun", 0, 0x201, be32(uint32(len(frames))), be32(off), sizes))
		return mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), traf)
	}
	moof := build(0)
	return append(build(uint32(len(moof)+8)), mkbox("mdat", data)...)
}

func sineFrames(t *testing.T) [][]byte {
	t.Helper()
	b, err := os.ReadFile("../gst/testdata/sine3s.adts")
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for len(b) >= 7 {
		n := int(b[3]&3)<<11 | int(b[4])<<3 | int(b[5])>>5
		frames = append(frames, b[7:n])
		b = b[n:]
	}
	return frames
}

func waitEvent(t *testing.T, h *Host, kind EventKind, track uint64, timeout time.Duration) Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-h.Events():
			if !ok {
				t.Fatalf("host exited: %v", h.Err())
			}
			if ev.Kind == EventError {
				t.Fatalf("error event: %s", ev.Err)
			}
			if ev.Kind == kind && ev.Track == track {
				return ev
			}
		case <-deadline:
			t.Fatalf("no event %d for track %d within %v", kind, track, timeout)
		}
	}
}

func TestPlayClearTrack(t *testing.T) {
	h := startHost(t)
	ctx := context.Background()
	frames := sineFrames(t)
	const per = 43 // ~1 s fragments
	var frags [][]byte
	for i := 0; i < len(frames); i += per {
		frags = append(frags, fragment(uint64(i)*1024, frames[i:min(i+per, len(frames))]))
	}

	if err := h.Load(ctx, 1, Media{Init: clearInit()}, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.Play(ctx); err != nil {
		t.Fatal(err)
	}
	for _, f := range frags {
		if err := h.Feed(ctx, 1, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.EndOfTrack(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if ev := waitEvent(t, h, EventStarted, 1, 5*time.Second); ev.Position > 500*time.Millisecond {
		t.Fatalf("started at %v", ev.Position)
	}
	waitEvent(t, h, EventEnded, 1, 6*time.Second)

	// Seek within the same init: start mid-fragment; earlier audio is clipped.
	if err := h.Load(ctx, 2, Media{}, 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := h.Feed(ctx, 1, frags[1]); !errors.Is(err, ErrStale) {
		t.Fatalf("feed for a superseded track: %v", err)
	}
	for _, f := range frags[1:] {
		if err := h.Feed(ctx, 2, f); err != nil {
			t.Fatal(err)
		}
	}
	ev := waitEvent(t, h, EventStarted, 2, 5*time.Second)
	if ev.Position < 1500*time.Millisecond || ev.Position > 2*time.Second {
		t.Fatalf("seek started at %v, want ~1.5s", ev.Position)
	}
	pos, ok, err := h.Pause(ctx)
	if err != nil || !ok || pos < 1500*time.Millisecond {
		t.Fatalf("pause at %v %v %v", pos, ok, err)
	}
}

// TestNextTrackAdvances queues a second track behind the first: the child
// reports the switch with EventAdvance (no EventEnded for the first) when
// the first one's trimmed audio has played.
func TestNextTrackAdvances(t *testing.T) {
	h := startHost(t)
	ctx := context.Background()
	frames := sineFrames(t)[:60] // ~1.4 s of access units each
	frag := fragment(0, frames)
	const dur = time.Second
	if err := h.Load(ctx, 1, Media{Init: clearInit(), Duration: dur}, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.Play(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Feed(ctx, 1, frag); err != nil {
		t.Fatal(err)
	}
	if err := h.EndOfTrack(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := h.Next(ctx, 2, Media{Init: clearInit(), Duration: dur}, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.Feed(ctx, 2, frag); err != nil {
		t.Fatal(err)
	}
	if err := h.EndOfTrack(ctx, 2); err != nil {
		t.Fatal(err)
	}
	started := waitEvent(t, h, EventStarted, 1, 5*time.Second)
	adv := waitEvent(t, h, EventAdvance, 2, 5*time.Second)
	if adv.Position > 100*time.Millisecond {
		t.Errorf("advanced at position %v of the next track, want ~0", adv.Position)
	}
	// The first track's second of audio played before the switch.
	if el := adv.At.Sub(started.At) + started.Position; el < dur-100*time.Millisecond || el > dur+200*time.Millisecond {
		t.Errorf("switch after %v of the first track, want ~%v", el, dur)
	}
	waitEvent(t, h, EventEnded, 2, 5*time.Second)
	// Too late to queue behind a track that has ended.
	if err := h.Next(ctx, 3, Media{Init: clearInit(), Duration: dur}, 0); !errors.Is(err, ErrEnded) {
		t.Fatalf("next after the end: %v, want ErrEnded", err)
	}
}

func TestChildExit(t *testing.T) {
	h := startHost(t)
	if err := h.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done not closed")
	}
	if !errors.Is(h.Err(), ErrExited) {
		t.Fatalf("Err = %v", h.Err())
	}
	if err := h.Play(context.Background()); !errors.Is(err, ErrExited) {
		t.Fatalf("call after exit: %v", err)
	}
}
