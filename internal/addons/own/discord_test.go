package own

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func track(id, title string) *cli.Track {
	return &cli.Track{
		ID: id, Title: title, Artist: "Artist " + title, Album: "Album " + title, Duration: 180,
		Artwork: "https://is1-ssl.mzstatic.com/image/thumb/Music/600x600bb.jpg",
	}
}

func playing(id, title string, position float64) cli.Status {
	return cli.Status{Running: true, State: "playing", Track: track(id, title), Position: position}
}

// A song that plays is the activity: its title, artist, album and artwork,
// and a start and end on the clock that put its position at now.
func TestDiscordPlanPlaying(t *testing.T) {
	var p discordPlan
	p.status(playing("1", "A", 30), t0)
	w, ok := p.want()
	if !ok || !w.Enabled || w.Activity == nil {
		t.Fatalf("playing: want = %+v, %v", w, ok)
	}
	a := w.Activity
	if a.Title != "A" || a.Artist != "Artist A" || a.Album != "Album A" || a.ArtworkURL == "" {
		t.Errorf("activity = %+v", a)
	}
	if want := t0.Add(-30 * time.Second); !a.Start.Equal(want) {
		t.Errorf("start = %v, want %v", a.Start, want)
	}
	if want := t0.Add(150 * time.Second); !a.End.Equal(want) {
		t.Errorf("end = %v, want %v (the song's 180 s from its start)", a.End, want)
	}
	// An unknown duration shows an elapsed time and no end.
	p.status(cli.Status{State: "playing", Track: &cli.Track{ID: "2", Title: "B"}, Position: 5}, t0)
	if w, _ := p.want(); w.Activity == nil || !w.Activity.End.IsZero() || w.Activity.Start.IsZero() {
		t.Errorf("no duration: %+v", w.Activity)
	}
}

// The activity is cleared while the song doesn't play, and left as it is
// while the next one loads, so it doesn't flicker.
func TestDiscordPlanStates(t *testing.T) {
	var p discordPlan
	p.status(playing("1", "A", 0), t0)
	if w, ok := p.want(); !ok || w.Activity == nil {
		t.Fatalf("playing: %+v %v", w, ok)
	}
	p.status(cli.Status{State: "loading", Track: track("2", "B")}, t0)
	if _, ok := p.want(); ok {
		t.Error("loading changed the presence")
	}
	for _, state := range []string{"paused", "ended", "error", "idle"} {
		st := playing("2", "B", 1)
		st.State = state
		p.status(st, t0)
		if w, ok := p.want(); !ok || !w.Enabled || w.Activity != nil {
			t.Errorf("%s: want = %+v, %v; cleared", state, w, ok)
		}
	}
	p.status(playing("2", "B", 1), t0)
	if w, ok := p.want(); !ok || w.Activity == nil || w.Activity.Title != "B" {
		t.Fatalf("playing again: %+v %v", w, ok)
	}
}

// A private listen or a preview is what Sona tells every add-on: nothing
// plays, an idle status and a now-playing without a track. Both clear the
// activity at once, and a listen after it shows again.
func TestDiscordPlanHiddenListenClears(t *testing.T) {
	var p discordPlan
	p.status(playing("1", "A", 10), t0)
	p.status(cli.Status{Running: true, State: "idle"}, t0.Add(time.Second))
	if w, ok := p.want(); !ok || w.Activity != nil {
		t.Fatalf("idle status: want = %+v, %v", w, ok)
	}

	p.status(playing("1", "A", 10), t0)
	p.nowPlaying(cli.NowPlaying{})
	if w, ok := p.want(); !ok || w.Activity != nil {
		t.Fatalf("now-playing without a track: want = %+v, %v", w, ok)
	}
	// A status of the old song that was already on its way doesn't bring it
	// back for long: the next status is what Sona says now.
	p.status(playing("2", "B", 0), t0.Add(2*time.Second))
	if w, ok := p.want(); !ok || w.Activity == nil || w.Activity.Title != "B" {
		t.Fatalf("next public listen: want = %+v, %v", w, ok)
	}
}

// A song now-playing names before any status has said how far it is shows
// nothing new: an activity with a start of a guess would be wrong.
func TestDiscordPlanWaitsForThePosition(t *testing.T) {
	var p discordPlan
	p.status(playing("1", "A", 100), t0)
	p.nowPlaying(cli.NowPlaying{Track: track("2", "B"), StartedAt: t0.UnixMilli()})
	if _, ok := p.want(); ok {
		t.Fatal("a new song with no position changed the presence")
	}
	// The same song again (its status came first) changes nothing.
	p.status(playing("2", "B", 0), t0.Add(time.Second))
	p.nowPlaying(cli.NowPlaying{Track: track("2", "B")})
	if w, ok := p.want(); !ok || w.Activity == nil || w.Activity.Title != "B" {
		t.Fatalf("status then now-playing: want = %+v, %v", w, ok)
	}
}

// Only https images on Apple's artwork CDN reach Discord, which fetches
// them itself: nothing that could carry credentials or point it elsewhere.
func TestDiscordArt(t *testing.T) {
	ok := "https://is1-ssl.mzstatic.com/image/thumb/Music/600x600bb.jpg"
	if got := discordArt(" " + ok + " "); got != ok {
		t.Errorf("discordArt(%q) = %q", ok, got)
	}
	for _, bad := range []string{
		"", "http://is1-ssl.mzstatic.com/a.jpg", "https://example.com/a.jpg", "https://mzstatic.com.evil.example/a.jpg",
		"https://.mzstatic.com/a.jpg", "https://user:pw@is1-ssl.mzstatic.com/a.jpg", "https://is1-ssl.mzstatic.com/a.jpg?token=1",
		"https://is1-ssl.mzstatic.com:8443/a.jpg", "https://is1-ssl.mzstatic.com/a.jpg#x", "/art/1.jpg", "file:///etc/passwd",
	} {
		if got := discordArt(bad); got != "" {
			t.Errorf("discordArt(%q) = %q, want rejected", bad, got)
		}
	}
}
