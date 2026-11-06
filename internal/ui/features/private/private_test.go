package private

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// rig is the pill over a full Env: the real PrivateService, its mirror kept by
// the module's Init.
type rig struct {
	h   *uitest.Harness
	m   module.Module
	tt  *uitest.Session
	svc *services.PrivateService
}

func newRig(t *testing.T, width int, opts ...uitest.Option) *rig {
	t.Helper()
	h := uitest.New(t, opts...)
	r := &rig{h: h, svc: services.Service[services.PrivateService](h.Env.Core)}
	r.m = New(h.Env)
	r.m.Init()
	r.tt = h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(8).AlignItems(ui.Center).Justify(ui.End).Children(func() { h.Env.PlayerControls.Build(c) })
	}, width, 200)
	return r
}

// settle runs what goroutines posted until the mirror answers.
func (r *rig) settle(t *testing.T) {
	t.Helper()
	r.tt.Until("the private state", func() bool {
		_, ok := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate)
		return ok
	})
}

// emit is Go reporting a state (private:changed).
func (r *rig) emit(s services.PrivateState) {
	r.h.Bus.Emit(services.EventPrivateChanged, s)
	r.tt.Drain()
}

func TestPillTogglesPrivateListeningAndFollowsGo(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	if r.svc.State().On {
		t.Fatal("private listening starts on")
	}
	// Off: the glyph alone, no word.
	if got := r.tt.Texts(); contains(got, "Private") {
		t.Errorf("an off pill shows the word: %q", got)
	}
	if err := r.tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Go to turn private listening on and the pill to follow", func() bool {
		return r.svc.State().On && contains(r.tt.Texts(), "Private")
	})
	if !r.svc.State().Saved {
		t.Error("the switch did not save the setting")
	}
	// On, then off again.
	if err := r.tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Go to turn private listening off", func() bool {
		return !r.svc.State().On && !contains(r.tt.Texts(), "Private")
	})
}

func TestPillFollowsChangesFromElsewhere(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	// The tray, the Settings row or `sona private` flips it: only the event tells.
	r.emit(services.PrivateState{On: true, Source: "setting", Saved: true})
	if !contains(r.tt.Texts(), "Private") {
		t.Errorf("private:changed did not turn the pill on: %q", r.tt.Texts())
	}
	r.emit(services.PrivateState{On: false, Source: "setting"})
	if contains(r.tt.Texts(), "Private") {
		t.Errorf("private:changed did not turn the pill off: %q", r.tt.Texts())
	}
}

func TestPillWordFollowsTheWindowWidth(t *testing.T) {
	on := services.PrivateState{On: true, Source: "setting", Saved: true}
	dev := services.PrivateState{On: true, Source: "development"}
	for _, c := range []struct {
		name  string
		width int
		state services.PrivateState
		word  string // "" for none
	}{
		{"narrow window: the tone alone", 1000, on, ""},
		{"from 1100: Private", 1100, on, "Private"},
		{"a development build below 1280 says Private", 1200, dev, "Private"},
		{"a development build from 1280 names itself", 1280, dev, "Private (development build)"},
		{"a development build, narrow", 1000, dev, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.width)
			r.settle(t)
			r.emit(c.state)
			var got string
			for _, s := range r.tt.Texts() {
				if s == "Private" || s == "Private (development build)" {
					got = s
				}
			}
			if got != c.word {
				t.Errorf("word %q, want %q (%q)", got, c.word, r.tt.Texts())
			}
		})
	}
}

func TestPillNameSaysWhereTheStateComesFrom(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	for _, c := range []struct {
		state services.PrivateState
		name  string
	}{
		{services.PrivateState{Source: "setting"}, "Private listening"},
		{services.PrivateState{On: true, Source: "development"}, "Private listening (development build)"},
		{services.PrivateState{On: true, Source: "environment"}, "Private listening (forced by SONA_PRIVATE=1)"},
	} {
		r.emit(c.state)
		if _, ok := r.tt.Find(c.name); !ok {
			t.Errorf("state %+v: no control named %q: %q", c.state, c.name, r.tt.Texts())
		}
	}
}

func TestPillDoesNothingWhileForcedOrUnknown(t *testing.T) {
	synctest.Test(t, doesNothingWhileForcedOrUnknown)
}

func doesNothingWhileForcedOrUnknown(t *testing.T) {
	// Forced: the mirror says SONA_PRIVATE=1 keeps it on (here Go's real state is off, so
	// a click that got through would turn it on).
	r := newRig(t, 1200)
	r.settle(t)
	r.emit(services.PrivateState{On: false, Source: "environment"})
	if err := r.tt.Click("Private listening (forced by SONA_PRIVATE=1)"); err != nil {
		t.Fatal(err)
	}
	r.tt.Quiesce() // a call that got through has run and posted by now
	if r.svc.State().On {
		t.Error("a click turned private listening on while it is forced")
	}

	// Unknown: Go has not answered (no Core to answer).
	h := uitest.New(t, uitest.Bare())
	m := New(h.Env)
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) { ui.Row(c).Fill().Children(func() { h.Env.PlayerControls.Build(c) }) }, 1200, 200)
	tt.Drain()
	if err := tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	tt.Quiesce()
	if _, ok := store.Peek[services.PrivateState](h.Env.Queries, store.KeyPrivate); ok {
		t.Error("a click made up a state")
	}
}

func TestSnapshotNeverReplacesANewerEvent(t *testing.T) {
	synctest.Test(t, snapshotNeverReplacesANewerEvent)
}

func snapshotNeverReplacesANewerEvent(t *testing.T) {
	h := uitest.New(t)
	New(h.Env).Init() // the snapshot runs on a goroutine and says off
	want := services.PrivateState{On: true, Source: "environment"}
	h.Bus.Emit(services.EventPrivateChanged, want)
	synctest.Wait() // the snapshot is posted behind it
	h.Main.Drain()
	if got, ok := store.Peek[services.PrivateState](h.Env.Queries, store.KeyPrivate); !ok || got != want {
		t.Errorf("mirror = %+v, %v; want the event's %+v", got, ok, want)
	}
}

func TestCommandIsUnavailableUntilKnownAndWhileForced(t *testing.T) {
	h := uitest.New(t, uitest.Bare())
	m := New(h.Env)
	cmd := m.Shortcuts[0]
	if cmd.ID != "private.toggle" || cmd.Group != "playback" || len(cmd.Keys) != 0 {
		t.Fatalf("command = %+v", cmd)
	}
	if cmd.When() {
		t.Error("available before Go reported the state")
	}
	set := func(s services.PrivateState) { store.Set(h.Env.Queries, store.KeyPrivate, s) }
	set(services.PrivateState{Source: "setting"})
	if !cmd.When() || cmd.Label() != "Turn on private listening" {
		t.Errorf("off: when %v, label %q", cmd.When(), cmd.Label())
	}
	set(services.PrivateState{On: true, Source: "development"})
	if !cmd.When() || cmd.Label() != "Turn off private listening" {
		t.Errorf("on in a development build: when %v, label %q", cmd.When(), cmd.Label())
	}
	set(services.PrivateState{On: true, Source: "environment"})
	if cmd.When() {
		t.Error("available while SONA_PRIVATE=1 forces it on")
	}
}

func TestCommandFlipsPrivateListening(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	cmd := r.m.Shortcuts[0]
	cmd.Run()
	r.tt.Until("private listening on", func() bool { return r.svc.State().On })
	r.tt.Until("the mirror to follow", func() bool {
		s, _ := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate)
		return s.On && cmd.Label() == "Turn off private listening"
	})
	cmd.Run()
	r.tt.Until("private listening off", func() bool { return !r.svc.State().On })
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestPillTooltipExplainsTheState(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	for _, c := range []struct {
		state services.PrivateState
		name  string
		tip   string
	}{
		{services.PrivateState{Source: "setting"}, "Private listening", "Turn on private listening"},
		{services.PrivateState{On: true, Source: "setting", Saved: true}, "Private listening", "Private listening is on: plays stay out of your listening history and recommendations, and aren't scrobbled, shown on Discord or added to History."},
		{services.PrivateState{On: true, Source: "development"}, "Private listening (development build)", "Private listening is on for this development build. Turning it off lasts until Sona quits."},
		{services.PrivateState{On: true, Source: "environment"}, "Private listening (forced by SONA_PRIVATE=1)", "On because Sona was started with SONA_PRIVATE=1"},
	} {
		r.emit(c.state)
		r.tt.Move(10, 190)
		r.tt.Advance(time.Second)
		pill, ok := r.tt.Find(c.name)
		if !ok {
			t.Fatalf("no %q: %q", c.name, r.tt.Texts())
		}
		r.tt.Move(pill.X+pill.W/2, pill.Y+pill.H/2)
		r.tt.Advance(time.Second)
		if !r.tt.HasText(c.tip) {
			t.Errorf("state %+v: no tooltip %q in %q", c.state, c.tip, r.tt.Texts())
		}
	}
}
