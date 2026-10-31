//go:build linux || (darwin && cgo)

package cdmhost

import (
	"encoding/gob"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// newTestServer is the child's request handler in-process, with a
// fakesink pipeline and no CDM (clear content only).
func newTestServer(t *testing.T) *server {
	t.Helper()
	if !gst.Have("avdec_aac") && !gst.Have("fdkaacdec") && !gst.Have("faad") {
		t.Skip("no AAC decoder installed")
	}
	if !gst.Have("equalizer-10bands") {
		t.Skip("equalizer-10bands not installed")
	}
	s := &server{
		log:    slog.New(slog.DiscardHandler),
		enc:    gob.NewEncoder(io.Discard),
		volume: 1,
		sink:   "fakesink",
		quit:   make(chan struct{}),
	}
	t.Cleanup(s.shutdown)
	return s
}

func (s *server) must(t *testing.T, r request) {
	t.Helper()
	if rep := s.handle(r); rep.Err != "" {
		t.Fatalf("%s: %s", r.Op, rep.Err)
	}
}

func (s *server) output(t *testing.T) gst.OutputSettings {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pipe == nil {
		t.Fatal("no pipeline")
	}
	out, ok := s.pipe.Output()
	if !ok {
		t.Fatal("pipeline has no live output")
	}
	return out
}

func TestEqualizerRequestsApplyAndOutliveThePipeline(t *testing.T) {
	s := newTestServer(t)
	eq := equalizer.Curve{Pregain: -6, Gains: [10]float64{6, 3}}
	// Sent before any pipeline exists: the first one starts with it.
	s.must(t, request{Op: opEqualizer, EQ: &eq})
	s.must(t, request{Op: opLoad, Track: 1, Data: clearInit()})
	if out := s.output(t); out.Bands != eq.Gains || math.Abs(out.Pregain+6) > 1e-4 {
		t.Fatalf("first pipeline output %+v", out)
	}
	// Changed live.
	eq = equalizer.Curve{Gains: [10]float64{9: -12}}
	s.must(t, request{Op: opEqualizer, EQ: &eq})
	if out := s.output(t); out.Bands != eq.Gains || math.Abs(out.Pregain) > 1e-4 {
		t.Fatalf("live change: %+v", out)
	}
	// A pipeline rebuilt after a failure keeps it.
	s.mu.Lock()
	old := s.pipe
	s.pipe = nil
	s.mu.Unlock()
	old.Close()
	s.must(t, request{Op: opLoad, Track: 2, Data: clearInit()})
	if out := s.output(t); out.Bands != eq.Gains {
		t.Fatalf("rebuilt pipeline output %+v", out)
	}
	if rep := s.handle(request{Op: opEqualizer}); rep.Err == "" {
		t.Error("equalizer request without settings accepted")
	}
}

func TestSoundCheckGainGoesToTheMixer(t *testing.T) {
	s := newTestServer(t)
	s.must(t, request{Op: opVolume, Volume: 0.5})
	s.must(t, request{Op: opLoad, Track: 1, Data: clearInit(), Gain: 0.25})
	gain := func() (float64, bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.pipe.TrackGain()
	}
	if g, on := gain(); g != 0.25 || on {
		t.Fatalf("Sound Check off: track gain %v applied %v, want 0.25 not applied", g, on)
	}
	s.must(t, request{Op: opTrackGain, Enabled: true})
	if g, on := gain(); g != 0.25 || !on {
		t.Fatalf("Sound Check on: track gain %v applied %v", g, on)
	}
	if v := s.output(t).Volume; v != 0.5 {
		t.Fatalf("volume %v: the gain is the mixer's, the user volume stays 0.5", v)
	}
	// A seek keeps the track's gain; the next track brings its own.
	s.must(t, request{Op: opLoad, Track: 2, Start: 1})
	if g, _ := gain(); g != 0.25 {
		t.Fatalf("after a seek: track gain %v", g)
	}
	s.must(t, request{Op: opLoad, Track: 3, Data: clearInit()})
	if g, _ := gain(); g != 0 {
		t.Fatalf("track without loudness: gain %v, want unity (0)", g)
	}
}
