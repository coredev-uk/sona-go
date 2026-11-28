package home

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func TestGreetingChangesAtTheBoundaries(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	day := func(h, m int) time.Time { return time.Date(2026, 3, 10, h, m, 0, 0, loc) }
	for _, c := range []struct {
		at         time.Time
		want       string
		next       time.Time
		nextWanted string
	}{
		{day(0, 0), "Good evening", day(5, 0), "Good morning"},
		{day(4, 59), "Good evening", day(5, 0), "Good morning"},
		{day(5, 0), "Good morning", day(12, 0), "Good afternoon"},
		{day(11, 59), "Good morning", day(12, 0), "Good afternoon"},
		{day(12, 0), "Good afternoon", day(18, 0), "Good evening"},
		{day(17, 59), "Good afternoon", day(18, 0), "Good evening"},
		{day(18, 0), "Good evening", day(29, 0), "Good morning"}, // 05:00 next day
		{day(23, 59), "Good evening", day(29, 0), "Good morning"},
	} {
		got, until := greetingAt(c.at, "")
		if got != c.want || !until.Equal(c.next) {
			t.Errorf("at %s: %q until %s, want %q until %s", c.at.Format("15:04"), got, until, c.want, c.next)
		}
		if after, _ := greetingAt(until, ""); after != c.nextWanted {
			t.Errorf("at %s: after the boundary %q, want %q", c.at.Format("15:04"), after, c.nextWanted)
		}
		// With a name it is the same greeting, said to them, at the same moments.
		named, namedUntil := greetingAt(c.at, "Alex")
		if named != c.want+", Alex" || !namedUntil.Equal(c.next) {
			t.Errorf("at %s with a name: %q until %s, want %q until %s", c.at.Format("15:04"), named, namedUntil, c.want+", Alex", c.next)
		}
	}
}

func TestHomeGreetsAboveTheHeadingAndFollowsTheSetting(t *testing.T) {
	now := time.Date(2026, 3, 10, 11, 59, 59, 0, time.Local)
	r := newRig(t, "/", 1132, 800, withHome(samplePage()))
	r.h.Env.Players.Now = func() time.Time { return now }
	r.until("the page", "For You", "Good morning")

	// The boundary: the page asked to be drawn again then, and says so.
	now = time.Date(2026, 3, 10, 12, 0, 1, 0, time.Local)
	r.until("the afternoon", "Good afternoon")
	requireNoTexts(t, r, "Good morning")

	// Off, the greeting is gone and the heading stays.
	off := r.h.Settings.Get()
	off.Home.Greeting = false
	store.Set(r.home.env.Queries, store.KeySettings, off)
	r.tt.Until("the greeting off", r.gone("Good afternoon"))
	requireTexts(t, r, "For You")
}

func TestHomeGreetsByNameWhenThereIsOneAndItIsAllowed(t *testing.T) {
	now := time.Date(2026, 3, 10, 19, 0, 0, 0, time.Local)
	r := newRig(t, "/", 1132, 800, withHome(samplePage()), withName("Alex"))
	r.h.Env.Players.Now = func() time.Time { return now }
	r.until("the named greeting", "For You", "Good evening, Alex")
	requireNoTexts(t, r, "Good evening", "Alex")

	// The profile changes (Go renewed it): the greeting follows.
	r.emit(services.EventProfileChanged, services.GreetingName{Name: "Sam"})
	r.until("the new name", "Good evening, Sam")
	requireNoTexts(t, r, "Good evening, Alex")

	// Sign-out clears it: no name, the plain greeting, never a handle.
	r.emit(services.EventProfileChanged, services.GreetingName{})
	r.until("no name", "Good evening")
	requireNoTexts(t, r, "Good evening, Sam")

	// The name comes back, then "Use my name" turns it off.
	r.emit(services.EventProfileChanged, services.GreetingName{Name: "Alex"})
	r.until("the name again", "Good evening, Alex")
	cfg := r.h.Settings.Get()
	cfg.Home.UseName = false
	store.Set(r.home.env.Queries, store.KeySettings, cfg)
	r.until("the plain greeting", "Good evening")
	requireNoTexts(t, r, "Good evening, Alex")

	// The greeting off takes the name with it, whatever "Use my name" says.
	cfg.Home.UseName, cfg.Home.Greeting = true, false
	store.Set(r.home.env.Queries, store.KeySettings, cfg)
	r.tt.Until("no greeting", r.gone("Good evening", "Good evening, Alex"))
	requireTexts(t, r, "For You")
}

func TestHomeGreetsWithoutANameWhenThereIsNoProfile(t *testing.T) {
	now := time.Date(2026, 3, 10, 8, 0, 0, 0, time.Local)
	r := newRig(t, "/", 1132, 800, withHome(samplePage()), withName(""))
	r.h.Env.Players.Now = func() time.Time { return now }
	r.until("the greeting", "For You", "Good morning")
	for _, s := range r.tt.Texts() {
		if strings.HasPrefix(s, "Good morning,") {
			t.Errorf("the greeting says %q with no profile", s)
		}
	}
}

func TestHomeDoesNotReadTheNameWhenItIsNotUsed(t *testing.T) {
	r := newRig(t, "/", 1132, 800, withHome(samplePage()), withName("Alex"),
		withSettings(func(s *config.Settings) { s.Home.UseName = false }))
	r.until("the page", "For You", "Top Picks for You")
	for _, s := range r.tt.Texts() {
		if strings.Contains(s, "Alex") {
			t.Fatalf("the name is on screen with Use my name off: %q", s)
		}
	}
	r.prof.mu.Lock()
	defer r.prof.mu.Unlock()
	if r.prof.reads != 0 {
		t.Errorf("the name was read %d times with Use my name off", r.prof.reads)
	}
}

func TestA11yGreetingIsReadWholeAndBeforeTheHeading(t *testing.T) {
	now := time.Date(2026, 3, 10, 19, 0, 0, 0, time.Local)
	r := newRig(t, "/", 1132, 800, withHome(samplePage()), withName("Alex"))
	r.h.Env.Players.Now = func() time.Time { return now }
	r.until("the page", "Good evening, Alex", "Top Picks for You")
	tr := access.Read(t, r.tt.Tester)

	// "For You" is still the page's level-1 heading, the one h1.
	h, ok := tr.Find(ui.RoleHeading, "For You")
	if !ok || h.Level != 1 {
		t.Fatalf("heading %v (found %v), want level 1", h, ok)
	}
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleHeading && n.Level == 1 && n.Label != "For You" {
			t.Errorf("a second level-1 heading %q", n.Label)
		}
	}

	// The greeting is one piece read before the heading; the name is never a
	// node of its own, and the greeting is no heading.
	greeting := -1
	for _, n := range tr.Nodes {
		switch n.Label {
		case "Good evening, Alex":
			greeting = n.Index
			if n.Role == ui.RoleHeading {
				t.Errorf("the greeting is a heading: %v", n)
			}
		case "Alex":
			t.Errorf("the name is read on its own: %v", n)
		}
	}
	if greeting < 0 {
		t.Fatalf("no node says the greeting; nodes %v", tr.Nodes)
	}
	if greeting > h.Index {
		t.Errorf("the greeting is node %d, after the heading at %d", greeting, h.Index)
	}
}
