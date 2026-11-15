package library

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// day is local midnight on a date.
func day(t *testing.T, date string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// addSong stores a song of album al (stored with the given release state when
// ok) as the sync does.
func addSong(t *testing.T, e *env, id, al string, date string, noPlayParams int64) {
	t.Helper()
	ctx := context.Background()
	ref, err := e.db.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
		LibraryID: db.NullString(id), Name: id, AlbumLibraryID: db.NullString(al), ReleaseDate: date, NoPlayParams: noPlayParams, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
}

func addAlbum(t *testing.T, e *env, id, date, catalog string, flagged int64) {
	t.Helper()
	ref, err := e.db.Q.UpsertLibraryAlbum(context.Background(), dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString(id), Name: id, ReleaseDate: date, CatalogReleaseDate: catalog, IsPrerelease: flagged, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Q.MarkLibraryItem(context.Background(), dbq.MarkLibraryItemParams{Kind: "album", Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
}

// Library.Song.Unreleased is the one answer to whether a song is out, and the
// lists hold exactly the songs it says are. A song goes by its own date, else
// its playParams, else its album's release state (the catalog album's date
// when the library album has none or the catalog's is still ahead; the album's
// isPrerelease only without a full date).
func TestSongUnreleased(t *testing.T) {
	ahead := day(t, "2026-11-20")
	before, atRelease := ahead.Add(-time.Hour), ahead
	const (
		noAlbum = "l.none"
	)
	albums := map[string]struct {
		date, catalog string
		flagged       int64
	}{
		"l.ahead":      {"", "2026-11-20", 1},           // pre-added: the catalog's date, flagged
		"l.aheadnoflg": {"", "2026-11-20", 0},           // the catalog's date alone
		"l.libdate":    {"2026-11-20", "", 0},           // a library date ahead
		"l.stale":      {"2020-01-01", "", 1},           // a date that has passed, a stale flag
		"l.cat":        {"2026-10-01", "2026-11-20", 1}, // the catalog's is ahead of the library's
		"l.yearflag":   {"", "2027", 1},                 // a year only, flagged
		"l.year":       {"", "2027", 0},                 // a year only
		"l.nodate":     {"", "", 1},                     // flagged without a date
		"l.out":        {"2020-01-01", "", 0},           // out
		"l.libyear":    {"2027", "2026-11-20", 0},       // the library has a year: the catalog's date is ahead
		"l.catfirst":   {"2026-11-20", "2026-12-31", 0}, // both ahead, the catalog's later
		"l.catpassed":  {"", "2026-10-01", 1},           // the catalog's date passed, still flagged: a year-less flag does not decide
	}
	tests := []struct {
		name         string
		album        string
		date         string
		noPlayParams int64
		now          time.Time
		want         bool // not out
	}{
		{"no playParams on an album ahead", "l.ahead", "", 1, before, true},
		{"no playParams, no date, the album's date has come", "l.ahead", "", 1, atRelease, false},
		{"the catalog's date alone", "l.aheadnoflg", "", 1, before, true},
		{"a library date ahead", "l.libdate", "", 1, before, true},
		{"a stale flag on a date that passed", "l.stale", "", 1, before, false},
		{"the catalog's date ahead of the library's", "l.cat", "", 1, before, true},
		{"a year only, flagged", "l.yearflag", "", 1, atRelease, true},
		{"a year only", "l.year", "", 1, atRelease, false},
		{"flagged, no date", "l.nodate", "", 1, atRelease, true},
		{"an album that is out", "l.out", "", 1, before, false},
		{"a library year, a catalog date ahead", "l.libyear", "", 1, before, true},
		{"both dates ahead", "l.catfirst", "", 1, atRelease, true},
		{"a catalog date passed, flagged, a full date", "l.catpassed", "", 1, before, false},
		{"no stored album", noAlbum, "", 1, before, false},
		// Apple gives some singles playParams and no releaseDate (Flower In A
		// Hurricane, GTA VI): the song is out whatever its album says.
		{"playParams but no date on an album ahead", "l.ahead", "", 0, before, false},
		{"a song out, on an album ahead", "l.ahead", "2026-09-17", 0, before, false},
		{"dated and out, no playParams", "l.ahead", "2026-09-17", 1, before, false},
		{"dated ahead", "l.out", "2026-12-01", 0, atRelease, true},
		{"dated ahead, no playParams", "l.out", "2026-12-01", 1, atRelease, true},
		{"a year only of its own", "l.ahead", "2027", 1, before, false},
		{"the last second before its date", "l.out", "2026-11-20", 0, ahead.Add(-time.Second), true},
		{"the first instant of its date", "l.out", "2026-11-20", 0, ahead, false},
	}
	e := newEnv(t)
	ctx := context.Background()
	for id, a := range albums {
		addAlbum(t, e, id, a.date, a.catalog, a.flagged)
	}
	for i, tc := range tests {
		addSong(t, e, fmt.Sprintf("i.%d", i), tc.album, tc.date, tc.noPlayParams)
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now
			e.lib.SetClock(func() time.Time { return now })
			id := fmt.Sprintf("i.%d", i)
			s, err := e.lib.Song(ctx, id)
			if err != nil || s.Unreleased != tc.want {
				t.Fatalf("Unreleased = %v (%v), want %v", s.Unreleased, err, tc.want)
			}
			// The album page's songs and the list say the same.
			if tc.album != noAlbum {
				_, tracks, err := e.lib.Album(ctx, tc.album)
				if err != nil {
					t.Fatal(err)
				}
				at := slices.IndexFunc(tracks, func(x Song) bool { return x.LibraryID == id })
				if at < 0 || tracks[at].Unreleased != tc.want {
					t.Errorf("the album's copy of the song = %+v", tracks)
				}
			}
			all, err := e.lib.AllSongs(ctx, ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if listed := slices.Contains(songIDs(all), id); listed == tc.want {
				t.Errorf("listed = %v for a song with Unreleased %v", listed, tc.want)
			}
		})
	}
}
