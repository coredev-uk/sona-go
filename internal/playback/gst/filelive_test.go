//go:build linux || (darwin && cgo)

package gst

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// liveMax mirrors filehead.c's LIVE_MAX: the most unread bytes a live
// stream may hold.
const liveMax = 8 << 20

func newLive(t *testing.T) *FilePipeline {
	t.Helper()
	if !Have("decodebin") {
		t.Skip("decodebin not installed")
	}
	p, err := NewFile(Options{Sink: "fakesink"}, Live)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// Sizes other than a length or Live are refused.
func TestNewFileLiveSizes(t *testing.T) {
	for _, size := range []int64{0, Live - 1} {
		if p, err := NewFile(Options{Sink: "fakesink"}, size); err == nil {
			p.Close()
			t.Errorf("size %d accepted", size)
		}
	}
}

// A live stream fed in uneven pieces (smaller and larger than one read)
// plays through in order and ends once complete, every byte read; it has
// no length and cannot seek.
func TestFileLivePlaysInOrderToEnd(t *testing.T) {
	for _, file := range []string{"sine2s.mp3", "sine3s.adts"} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + file)
			if err != nil {
				t.Fatal(err)
			}
			p := newLive(t)
			sizes := []int{100, 9000, 1, 4096, 20000}
			off := 0
			for i := 0; off < len(data); i++ {
				end := min(off+sizes[i%len(sizes)], len(data))
				if err := p.Feed(int64(off), data[off:end]); err != nil {
					t.Fatal(err)
				}
				off = end
			}
			// Out of order is refused.
			if err := p.Feed(int64(off+1), []byte{0}); err == nil {
				t.Error("feed past the end of the data accepted")
			}
			if err := p.Pause(); err != nil {
				t.Fatal(err)
			}
			waitFileEvent(t, p, EventAsyncDone)
			if have, size, _ := p.Level(); have != int64(len(data)) || size != 0 {
				t.Errorf("level have %d size %d, want %d and 0", have, size, len(data))
			}
			if err := p.Seek(500 * time.Millisecond); err == nil {
				t.Error("seek in a live stream succeeded")
			}
			p.Complete()
			if err := p.Feed(int64(len(data)), []byte{0}); err == nil {
				t.Error("feed after completion accepted")
			}
			if err := p.Play(); err != nil {
				t.Fatal(err)
			}
			// Played in real time: up to 3 s of audio.
			deadline := time.After(6 * time.Second)
			for eos := false; !eos; {
				select {
				case ev := <-p.Events():
					if ev.Kind == EventError {
						t.Fatal(ev.Err)
					}
					eos = ev.Kind == EventEOS
				case <-deadline:
					t.Fatal("no end of stream")
				}
			}
			if have, _, pos := p.Level(); pos != have {
				t.Errorf("read %d of %d bytes by the end", pos, have)
			}
		})
	}
}

// A live stream holds at most liveMax unread bytes; each byte read frees
// room for another.
func TestFileLiveCapFreesAsRead(t *testing.T) {
	mp3, err := os.ReadFile("testdata/sine2s.mp3")
	if err != nil {
		t.Fatal(err)
	}
	// Concatenated MP3s are one longer MP3 stream.
	n := liveMax / len(mp3)
	stream := bytes.Repeat(mp3, n+2)
	p := newLive(t)
	full := n * len(mp3)
	if err := p.Feed(0, stream[:full]); err != nil {
		t.Fatal(err)
	}
	if err := p.Feed(int64(full), stream[full:liveMax]); err != nil {
		t.Fatalf("filling to the cap: %v", err)
	}
	if err := p.Feed(liveMax, stream[liveMax:liveMax+1]); err == nil {
		t.Fatal("feed past the cap accepted")
	}
	if have, _, _ := p.Level(); have != liveMax {
		t.Fatalf("have %d after a refused feed, want %d", have, liveMax)
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "the decoder to read a whole MP3", func() bool {
		_, _, pos := p.Level()
		return pos >= int64(len(mp3))
	})
	// What was read is dropped: the same amount fits again.
	if err := p.Feed(liveMax, stream[liveMax:liveMax+len(mp3)]); err != nil {
		t.Errorf("feed after reads: %v", err)
	}
}
