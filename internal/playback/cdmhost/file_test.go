//go:build linux || (darwin && cgo)

package cdmhost

import (
	"encoding/binary"
	"encoding/gob"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// eventServer is the child's handler in-process with its monitor running
// and its events delivered on a channel.
func eventServer(t *testing.T) (*server, <-chan Event) {
	t.Helper()
	r, w := io.Pipe()
	s := &server{
		log:    slog.New(slog.DiscardHandler),
		enc:    gob.NewEncoder(w),
		volume: 1,
		sink:   "fakesink",
		quit:   make(chan struct{}),
	}
	events := make(chan Event, 64)
	go func() {
		dec := gob.NewDecoder(r)
		for {
			var f frame
			if dec.Decode(&f) != nil {
				return
			}
			if f.Event != nil {
				events <- *f.Event
			}
		}
	}()
	go s.monitor()
	t.Cleanup(func() {
		close(s.quit)
		s.shutdown()
		_ = w.Close()
	})
	return s, events
}

func waitChildEvent(t *testing.T, events <-chan Event, kind EventKind, track uint64, within time.Duration) Event {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case ev := <-events:
			if ev.Kind == EventError {
				t.Fatalf("error event: %s", ev.Err)
			}
			if ev.Kind == kind && ev.Track == track {
				return ev
			}
		case <-deadline:
			t.Fatalf("no event %d for track %d within %v", kind, track, within)
		}
	}
}

// padMoov grows the moov box of an MP4 whose moov comes last (after mdat)
// by a free box of n bytes, as large tags (cover art) do: the demuxer then
// reads one moov bigger than any single read, from the file's end.
func padMoov(t *testing.T, data []byte, n int) []byte {
	t.Helper()
	for i := 0; i+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[i:]))
		if size < 8 {
			break
		}
		if string(data[i+4:i+8]) == "moov" {
			if i+size != len(data) {
				t.Fatal("moov is not the last box")
			}
			out := append(data[:len(data):len(data)], make([]byte, n)...)
			binary.BigEndian.PutUint32(out[i:], uint32(size+n))
			binary.BigEndian.PutUint32(out[len(data):], uint32(n))
			copy(out[len(data)+4:], "free")
			return out
		}
		i += size
	}
	t.Fatal("no moov box")
	return nil
}

// Uploads: each format decodes through the child's file path, fed in
// chunks while it plays, then seeks within the bytes it has.
func TestUploadFormatsPlayAndSeek(t *testing.T) {
	if !gst.Have("decodebin") {
		t.Skip("decodebin not installed")
	}
	for _, c := range []struct {
		file string
		dur  time.Duration
		pad  int // bytes added to a trailing moov (padMoov)
	}{
		{"sine2s.mp3", 2069 * time.Millisecond, 0},
		{"sine2s.m4a", 2027 * time.Millisecond, 0},
		{"sine2s-alac.m4a", 2183 * time.Millisecond, 0},
		// Not "fast start": the movie box, larger than any one read, sits
		// after the audio and arrives last.
		{"sine2s-moovend.m4a", 2000 * time.Millisecond, 512 << 10},
	} {
		t.Run(c.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}
			if c.pad > 0 {
				data = padMoov(t, data, c.pad)
			}
			s, events := eventServer(t)
			s.must(t, request{Op: opLoadFile, Track: 1, Size: int64(len(data))})
			s.must(t, request{Op: opPlay})
			// The first bytes arrive slowly: playback waits for them.
			const chunk = 4096
			for off := 0; off < len(data); off += chunk {
				end := min(off+chunk, len(data))
				s.must(t, request{Op: opFileData, Track: 1, Offset: int64(off), Data: data[off:end]})
				if off < 3*chunk {
					time.Sleep(20 * time.Millisecond)
				}
			}
			s.must(t, request{Op: opFileEnd, Track: 1})
			ev := waitChildEvent(t, events, EventStarted, 1, 5*time.Second)
			for ev.Duration == 0 {
				ev = waitChildEvent(t, events, EventPosition, 1, time.Second)
			}
			if d := ev.Duration - c.dur; d < -50*time.Millisecond || d > 50*time.Millisecond {
				t.Errorf("duration %v, want ≈%v", ev.Duration, c.dur)
			}
			// Bytes for another generation are refused.
			if rep := s.handle(request{Op: opFileData, Track: 9, Offset: int64(len(data))}); rep.Code != codeStale {
				t.Errorf("stale file data: %+v", rep)
			}

			// Seek (a Load without init) to 1 s: plays from there to the end.
			t0 := time.Now()
			s.must(t, request{Op: opLoad, Track: 2, Start: time.Second})
			ev = waitChildEvent(t, events, EventStarted, 2, 3*time.Second)
			if ev.Position < time.Second || ev.Position > 1300*time.Millisecond {
				t.Errorf("after the seek, playing from %v", ev.Position)
			}
			waitChildEvent(t, events, EventEnded, 2, 3*time.Second)
			if took := time.Since(t0); took > c.dur-time.Second+700*time.Millisecond {
				t.Errorf("seek to 1 s then the end took %v of a %v file", took, c.dur)
			}
		})
	}
}

// An upload started part-way (a resume) begins there.
func TestUploadStartsAtOffset(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", "sine2s.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	s, events := eventServer(t)
	s.must(t, request{Op: opLoadFile, Track: 1, Size: int64(len(data)), Start: 1500 * time.Millisecond})
	s.must(t, request{Op: opFileData, Track: 1, Data: data})
	s.must(t, request{Op: opFileEnd, Track: 1})
	s.must(t, request{Op: opPlay})
	ev := waitChildEvent(t, events, EventStarted, 1, 5*time.Second)
	if ev.Position < 1500*time.Millisecond || ev.Position > 1800*time.Millisecond {
		t.Errorf("started at %v, want 1.5 s", ev.Position)
	}
	waitChildEvent(t, events, EventEnded, 1, 2*time.Second)
}

// An upload resumed past its end (a stale position) ends at once, without
// waiting for the parent's end-of-file, and its output can still move.
func TestUploadStartPastEndEnds(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", "sine2s.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	s, events := eventServer(t)
	s.must(t, request{Op: opLoadFile, Track: 1, Size: int64(len(data)), Start: 20 * time.Second})
	s.must(t, request{Op: opFileData, Track: 1, Data: data})
	s.must(t, request{Op: opPlay})
	waitChildEvent(t, events, EventEnded, 1, 3*time.Second)
	t0 := time.Now()
	s.must(t, request{Op: opDevice, Device: "elsewhere"})
	if took := time.Since(t0); took > time.Second {
		t.Errorf("moving the output took %v", took)
	}
}

// A live stream (a broadcaster's ADTS AAC) plays through the file path fed
// while it plays, with no duration, and ends once the parent ends it.
func TestLiveFilePlaysToEnd(t *testing.T) {
	if !gst.Have("decodebin") {
		t.Skip("decodebin not installed")
	}
	data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", "sine3s.adts"))
	if err != nil {
		t.Fatal(err)
	}
	s, events := eventServer(t)
	s.must(t, request{Op: opLoadFile, Track: 1, Size: gst.Live})
	s.must(t, request{Op: opPlay})
	const chunk = 4096
	for off := 0; off < len(data); off += chunk {
		end := min(off+chunk, len(data))
		s.must(t, request{Op: opFileData, Track: 1, Offset: int64(off), Data: data[off:end]})
	}
	ev := waitChildEvent(t, events, EventStarted, 1, 5*time.Second)
	if ev.Position > 300*time.Millisecond {
		t.Errorf("started at %v, want the start", ev.Position)
	}
	s.must(t, request{Op: opFileEnd, Track: 1})
	waitChildEvent(t, events, EventEnded, 1, 5*time.Second)
}
