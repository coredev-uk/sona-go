//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// waitHostEvent returns h's next event of kind for track, skipping others.
func waitHostEvent(t *testing.T, h *Host, kind EventKind, track uint64, within time.Duration) Event {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case ev, ok := <-h.Events():
			if !ok {
				t.Fatalf("host exited waiting for event %d of track %d: %v", kind, track, h.Err())
			}
			if ev.Kind == EventError {
				t.Fatalf("host error: %s", ev.Err)
			}
			if ev.Kind == kind && ev.Track == track {
				return ev
			}
		case <-deadline:
			t.Fatalf("no event %d for track %d within %v", kind, track, within)
		}
	}
}

// A clear host (previews, uploads) starts with no CDM at all, even with a
// CDM path that does not exist, plays a plain file to its end, and refuses
// every CDM request.
func TestClearHostPlaysWithoutACDM(t *testing.T) {
	if !gst.Have("decodebin") {
		t.Skip("decodebin not installed")
	}
	data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", "sine2s.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := Start(ctx, Options{Clear: true, CDMPath: "/nonexistent/libwidevinecdm.so", Sink: "fakesink"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	info := h.Info()
	if info.CDMPath != "" || info.CDMVersion != "" || info.CDMInterface != 0 {
		t.Errorf("a clear host reported a CDM: %+v", info)
	}
	if info.PID == 0 || info.GStreamer == "" || info.Sink != "fakesink" {
		t.Errorf("incomplete clear host info: %+v", info)
	}

	if err := h.LoadFile(ctx, 1, int64(len(data)), 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.FeedFile(ctx, 1, 0, data); err != nil {
		t.Fatal(err)
	}
	if err := h.EndOfFile(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := h.Play(ctx); err != nil {
		t.Fatal(err)
	}
	waitHostEvent(t, h, EventStarted, 1, 10*time.Second)
	waitHostEvent(t, h, EventEnded, 1, 10*time.Second)

	if _, _, err := h.CreateSession(ctx, make([]byte, 16)); err == nil {
		t.Error("a clear host opened a CDM session")
	}
	if err := h.SetServerCertificate(ctx, []byte("cert")); err == nil {
		t.Error("a clear host took a service certificate")
	}
}

// Probe with Clear reports the audio stack without a CDM.
func TestProbeClear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := Probe(ctx, Options{Clear: true, Sink: "fakesink"})
	if err != nil {
		t.Fatal(err)
	}
	if info.PID == 0 || info.GStreamer == "" || info.Decoder == "" || info.Sink != "fakesink" || info.CDMPath != "" || info.CDMInterface != 0 {
		t.Fatalf("clear probe = %+v", info)
	}
}
