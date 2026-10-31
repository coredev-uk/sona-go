package scrobble

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// A listen private at any point since it began (private mode on at its
// start, turned on during it, or on while it was paused) is neither
// announced nor scrobbled; the listen after private mode turns off is.
func TestTrackerSkipsPrivateListens(t *testing.T) {
	clock := t0
	m := privatemode.New(privatemode.Options{Now: func() time.Time { return clock }})
	tr := Tracker{private: m.Since}
	set := func(at int, on bool) {
		t.Helper()
		clock = t0.Add(sec(at))
		if _, err := m.Set(on); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(s step) (bool, bool) {
		clock = t0.Add(sec(s.at))
		now, done := tr.Observe(coordinator.Activity{
			Kind: s.kind, Item: s.item, At: t0.Add(sec(s.at)),
			Position: sec(s.pos), From: sec(s.from), Duration: s.item.Duration, Reason: s.reason,
		})
		return now != nil, done != nil
	}
	nat, paused := coordinator.EndNatural, coordinator.EndPaused
	a, b, c, d := song("a", sec(180)), song("b", sec(180)), song("c", sec(180)), song("d", sec(180))

	// a: public start, private mode on mid-listen → not scrobbled.
	if now, _ := observe(start(a, 0, 0)); !now {
		t.Fatal("a: public start not announced")
	}
	set(30, true)
	if _, done := observe(end(a, 180, 180, nat, true)); done {
		t.Error("a: scrobbled after private mode turned on mid-listen")
	}

	// b: started private, private mode off before it ends → still private.
	if now, _ := observe(start(b, 200, 0)); now {
		t.Error("b: private start announced")
	}
	set(250, false)
	if _, done := observe(end(b, 380, 180, nat, true)); done {
		t.Error("b: a private listen was scrobbled after private mode turned off")
	}

	// c: the next listen is public again.
	if now, _ := observe(start(c, 400, 0)); !now {
		t.Error("c: not announced after private mode turned off")
	}
	if _, done := observe(end(c, 580, 180, nat, true)); !done {
		t.Error("c: public listen not scrobbled")
	}

	// d: private mode on and off again while d is paused: its resume is
	// the same listen, so it stays private.
	observe(start(d, 600, 0))
	observe(end(d, 700, 100, paused, true))
	set(710, true)
	set(720, false)
	if now, _ := observe(start(d, 730, 100)); now {
		t.Error("d: resume of a listen that was private announced")
	}
	if _, done := observe(end(d, 810, 180, nat, true)); done {
		t.Error("d: resumed private listen scrobbled")
	}
}

func TestScrobblerQueuesNothingWhilePrivate(t *testing.T) {
	m := privatemode.New(privatemode.Options{Override: privatemode.OverrideOn})
	p := &fakeProvider{name: "lastfm"}
	s := New(Options{Providers: []Provider{p}, Backoff: fastBackoff, Private: m.Since})
	defer s.Close()
	connect(s, "lastfm", "good")
	s.Start()
	for i := range 3 {
		play(s, i)
	}
	if n := s.State("lastfm").Pending; n != 0 {
		t.Errorf("%d private listens queued", n)
	}
	// Give delivery a moment: nothing may be announced or sent.
	time.Sleep(50 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.now) != 0 || len(p.got) != 0 {
		t.Errorf("sent %d announcements and %d batches while private", len(p.now), len(p.got))
	}
}
