package home

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/store"
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
		got, until := greetingAt(c.at)
		if got != c.want || !until.Equal(c.next) {
			t.Errorf("at %s: %q until %s, want %q until %s", c.at.Format("15:04"), got, until, c.want, c.next)
		}
		if after, _ := greetingAt(until); after != c.nextWanted {
			t.Errorf("at %s: after the boundary %q, want %q", c.at.Format("15:04"), after, c.nextWanted)
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
