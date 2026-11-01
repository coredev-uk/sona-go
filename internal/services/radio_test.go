package services

import (
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// A live programme's art is Apple's 4:1 key art with a "bb" crop, which
// fits the whole banner into any box. The schedule shows it as Apple's
// apps do: the centre square ("cc"), square at every size, cut by the CDN
// from the full image.
func TestScheduleArtIsCentreSquare(t *testing.T) {
	m := newMapper(t)
	s := &RadioService{e: &Env{m: m}}
	const show = "https://is1-ssl.mzstatic.com/image/thumb/Features/v4/90/show.png/"
	now := time.Date(2026, 11, 1, 20, 30, 0, 0, time.UTC)
	event := func(id, start, end string, art *applemusic.Artwork) applemusic.Resource[applemusic.StationEventAttributes] {
		ev := applemusic.Resource[applemusic.StationEventAttributes]{ID: id, Type: "station-events"}
		ev.Attributes.Title = id
		ev.Attributes.StartTime, ev.Attributes.EndTime = start, end
		ev.Attributes.Artwork = art
		return ev
	}
	var st applemusic.Station
	st.Relationships.Events.Data = []applemusic.Resource[applemusic.StationEventAttributes]{
		event("next", "2026-11-01T22:00:00Z", "2026-11-02T00:00:00Z", nil),
		event("on-air", "2026-11-01T20:00:00Z", "2026-11-01T22:00:00Z",
			&applemusic.Artwork{URL: show + "{w}x{h}bb.{f}", Width: 4320, Height: 1080}),
		event("ended", "2026-11-01T18:00:00Z", "2026-11-01T20:00:00Z",
			&applemusic.Artwork{URL: show + "{w}x{h}bb.{f}", Width: 4320, Height: 1080}),
	}

	got := s.schedule(st, now)
	if len(got) != 2 || got[0].ID != "on-air" || got[1].ID != "next" {
		t.Fatalf("schedule = %+v, want on-air then next", got)
	}
	art := got[0].Art
	for _, u := range []struct{ got, want string }{
		{art.Small, m.art.URL(show+"{w}x{h}cc.{f}", 96, 96)},
		{art.Medium, m.art.URL(show+"{w}x{h}cc.{f}", 400, 400)},
		{art.Large, m.art.URL(show+"{w}x{h}cc.{f}", 1000, 1000)},
	} {
		if u.got != u.want || !strings.HasPrefix(u.got, "/art/") {
			t.Errorf("art URL = %q, want %q (the centre square)", u.got, u.want)
		}
	}
	if got[1].Art != (Art{}) {
		t.Errorf("programme without artwork: art = %+v", got[1].Art)
	}
}
