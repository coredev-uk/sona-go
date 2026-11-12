package album

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenAlbum is a catalogue album with everything the page shows: badges,
// a popular song, an explicit one, fine print, notes and a shelf.
func goldenAlbum(p *services.CatalogAlbum) {
	p.Album.Explicit = true
	p.Album.Traits = services.Traits{Lossless: true, Atmos: true}
	p.Tracks[3].Explicit = true
	p.Header = services.Header{Backdrop: "/art/100/backdrop.jpg", Colors: []string{"#c4793a"}}
	p.MoreByArtist = []services.Album{
		{ID: "200", Name: "Blue Hour", Artist: "Mia Vale", ReleaseDate: "2021-05-01", Art: services.Art{Medium: "/art/200/m.jpg"}},
		{ID: "201", Name: "Dust", Artist: "Mia Vale", ReleaseDate: "2017-02-10", Art: services.Art{Medium: "/art/201/m.jpg"}},
	}
}

// goldenRegional writes dates and durations as an en-GB system does, as the
// reference shots do.
func goldenRegional(t *testing.T) {
	i18n.SetRegional("en-GB")
	t.Cleanup(func() { i18n.SetRegional() })
}

// The goldens are the shell's page column: 1132×1253 less the sidebar (233),
// the top bar (48) and the player bar (72) is 899×1133; the smallest window,
// 940×600, is 707×480.
func TestGoldenAlbumPage(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-light", nil},
		{"album-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goldenRegional(t)
			r := newRig(t, "/album/100", 899, 1133, c.opts...)
			r.addCatalog(saltFlats(goldenAlbum))
			r.shows("Salt Flats", "Blue Hour", "Notes")
			r.tt.Art = append(r.tt.Art, r.h.Env.Art)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

// The gatefold's wide layout: 1440×900 less the sidebar, top bar and player
// bar is 1207×780 of page column.
func TestGoldenAlbumPageWide(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-wide-light", nil},
		{"album-wide-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goldenRegional(t)
			r := newRig(t, "/album/100", 1207, 780, c.opts...)
			r.addCatalog(saltFlats(goldenAlbum))
			r.shows("Salt Flats", "Blue Hour", "Notes")
			r.tt.Art = append(r.tt.Art, r.h.Env.Art)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

func TestGoldenAlbumPageMinimumWindow(t *testing.T) {
	goldenRegional(t)
	r := newRig(t, "/album/100", 707, 480)
	r.addCatalog(saltFlats(goldenAlbum))
	r.shows("Salt Flats", "Notes")
	r.tt.Art = append(r.tt.Art, r.h.Env.Art)
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "album-940x600-light")
}

// A pre-added library album, opened by its library ID, takes its linked
// catalogue album's release state: the countdown, and only the songs out
// playable.
func TestGoldenLibraryAlbumNotOutYet(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"library-album-not-out-light", nil},
		{"library-album-not-out-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goldenRegional(t)
			r := newRig(t, "/album/l.9", 899, 1133, c.opts...)
			r.addPreAddedLibraryAlbum(r.now.Add(5*time.Hour + 12*time.Minute))
			r.shows("Salt Flats", "Out in 5 hours, 12 minutes")
			r.tt.Art = append(r.tt.Art, r.h.Env.Art)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

// goldenUpcoming is an album not out for 18 days and 6 hours, two of its five
// songs out now; goldenUpcomingYear one Apple gives only the year of, none
// of its songs out. upcomingRig's platform has notifications, so Remind me
// shows.
func goldenUpcoming(r *rig) func(p *services.CatalogAlbum) {
	release := r.now.Add(18*24*time.Hour + 6*time.Hour)
	return func(p *services.CatalogAlbum) {
		goldenAlbum(p)
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
		for i := 2; i < 5; i++ {
			p.Tracks[i].Playable, p.Tracks[i].DurationMs, p.Tracks[i].Artist = false, 0, "unknown"
		}
	}
}

func goldenUpcomingYear(p *services.CatalogAlbum) {
	goldenAlbum(p)
	p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, 0, "2027"
	for i := range p.Tracks {
		p.Tracks[i].Playable, p.Tracks[i].DurationMs, p.Tracks[i].Artist = false, 0, "unknown"
	}
}

func upcomingRig(t *testing.T, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	goldenRegional(t)
	opts = append(opts, uitest.Platform(func(p *services.Platform) {
		p.Notifications = func(context.Context, func(string)) (services.Notifier, error) { return nil, errors.ErrUnsupported }
	}))
	return newRig(t, "/album/100", w, h, opts...)
}

func settleGolden(t *testing.T, r *rig, name string) {
	t.Helper()
	r.tt.Art = append(r.tt.Art, r.h.Env.Art)
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), name)
}

func TestGoldenUpcomingAlbumPage(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-upcoming-light", nil},
		{"album-upcoming-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := upcomingRig(t, 899, 1133, c.opts...)
			r.addCatalog(saltFlats(goldenUpcoming(r)))
			r.shows("Out in 18 days, 6 hours", "Remind me", "Notes")
			settleGolden(t, r, c.name)
		})
	}
}

func TestGoldenUpcomingAlbumPageMinimumWindow(t *testing.T) {
	r := upcomingRig(t, 707, 480)
	r.addCatalog(saltFlats(goldenUpcoming(r)))
	r.shows("Out in 18 days, 6 hours", "Remind me")
	settleGolden(t, r, "album-upcoming-940x600-light")
}

// With no day and no song out, Pre-add is the page's one primary action.
func TestGoldenUpcomingYearOnlyAlbumPage(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-upcoming-year-light", nil},
		{"album-upcoming-year-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := upcomingRig(t, 899, 1133, c.opts...)
			r.addCatalog(saltFlats(goldenUpcomingYear))
			r.shows("Out 2027", "Pre-add", "Notes")
			settleGolden(t, r, c.name)
		})
	}
}

// What the account has done about it: pre-added, with a reminder set.
func TestGoldenUpcomingAlbumPageDone(t *testing.T) {
	r := upcomingRig(t, 899, 1133)
	r.addCatalog(saltFlats(goldenUpcoming(r)))
	r.states.inLib["100"] = true
	store.Set(r.h.Env.Queries, store.KeyReminders, []services.Reminder{{AlbumID: "100"}})
	r.shows("Out in 18 days, 6 hours", "Notes")
	r.tt.Until("both done", r.tt.Shows("Sunday, 22 November 2026 · Pre-added · Reminder set"))
	settleGolden(t, r, "album-upcoming-done-light")
}
