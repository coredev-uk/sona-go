//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// misbehave is a compromised child: it writes a frame announcing 512 MiB
// ("big-frame"), or floods position samples and then other events
// ("flood"), and then waits to be killed.
func misbehave(mode string) {
	switch mode {
	case "big-frame":
		_, _ = os.Stdout.Write([]byte{0xfc, 0x20, 0, 0, 0}) // gob length 0x20000000
	case "flood":
		enc := gob.NewEncoder(os.Stdout)
		for range 3 * maxQueuedEvents {
			_ = enc.Encode(frame{Event: &Event{Kind: EventPosition, Track: 1, Position: time.Second}})
		}
		for range 2 * maxQueuedEvents {
			_ = enc.Encode(frame{Event: &Event{Kind: EventBuffering, Track: 1, Buffering: true}})
		}
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

// spawnMisbehaving starts a misbehaving child and waits for the parent to
// give up on it.
func spawnMisbehaving(t *testing.T, mode string) *Host {
	t.Helper()
	t.Setenv(envTestChild, mode)
	h, err := spawn(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	select {
	case <-h.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the parent kept reading the child")
	}
	// Killed: signalled on Unix, exit code 1 (TerminateProcess) on Windows;
	// the child itself only exits, with 0, after a minute.
	if ws := h.cmd.ProcessState; ws == nil || ws.Success() {
		t.Errorf("child was not killed: %v", ws)
	}
	return h
}

// F8: a frame larger than maxChildFrame ends the child before the parent
// allocates for it.
func TestOversizedFrameKillsChild(t *testing.T) {
	h := spawnMisbehaving(t, "big-frame")
	if err := h.Err(); !errors.Is(err, ErrExited) || !errors.Is(err, errMisbehaved) || !strings.Contains(err.Error(), "frame") {
		t.Fatalf("Err = %v", err)
	}
}

// F8: queued events stay bounded: position samples beyond the bound are
// dropped, any other event beyond it ends the child.
func TestEventFloodIsBounded(t *testing.T) {
	h := spawnMisbehaving(t, "flood")
	if err := h.Err(); !errors.Is(err, errMisbehaved) || !strings.Contains(err.Error(), "events queued") {
		t.Fatalf("Err = %v", err)
	}
	h.evMu.Lock()
	n := len(h.evq)
	h.evMu.Unlock()
	if n > maxQueuedEvents {
		t.Fatalf("%d events queued, bound %d", n, maxQueuedEvents)
	}
}

// gobFrames passes legitimate gob streams through unchanged, whatever the
// read sizes.
func TestGobFramesPassThrough(t *testing.T) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	want := []frame{
		{Reply: &reply{ID: 1, Challenge: bytes.Repeat([]byte{1}, 300)}},
		{Event: &Event{Kind: EventMessage, Data: bytes.Repeat([]byte{2}, 70_000)}},
		{Reply: &reply{ID: 2}},
	}
	for _, f := range want {
		if err := enc.Encode(f); err != nil {
			t.Fatal(err)
		}
	}
	dec := gob.NewDecoder(&gobFrames{r: bufio.NewReaderSize(io.LimitReader(&buf, int64(buf.Len())), 16), max: 1 << 20})
	for i, w := range want {
		var f frame
		if err := dec.Decode(&f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if (f.Reply == nil) != (w.Reply == nil) || f.Event != nil && len(f.Event.Data) != len(w.Event.Data) {
			t.Fatalf("frame %d = %+v", i, f)
		}
	}
	// The same stream with a limit below the 70 kB message fails at it.
	buf.Reset()
	enc = gob.NewEncoder(&buf)
	for _, f := range want {
		_ = enc.Encode(f)
	}
	dec = gob.NewDecoder(&gobFrames{r: bufio.NewReader(&buf), max: 64 << 10})
	var err error
	for range want {
		var f frame
		if err = dec.Decode(&f); err != nil {
			break
		}
	}
	if !errors.Is(err, errMisbehaved) {
		t.Fatalf("over the limit: %v", err)
	}
}
