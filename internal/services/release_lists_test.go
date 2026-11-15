package services

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// A song with neither playParams nor a date of its own goes by its album, and
// the album page's countdown (libraryRelease, in the mapper) is the album's
// own answer: the library's decision for such a song (in SQL) must be the
// album's, for every release state an album can store and across its day.
func TestUndatedPlaceholderSongFollowsItsAlbumsCountdown(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	albums := []struct {
		date, catalog string
		flagged       int64
	}{
		{"", "2026-11-20", 1},
		{"", "2026-11-20", 0},
		{"2026-11-20", "", 0},
		{"2020-01-01", "", 1},
		{"2026-10-01", "2026-11-20", 1},
		{"", "2027", 1},
		{"", "2027", 0},
		{"", "", 1},
		{"", "", 0},
		{"2020-01-01", "", 0},
		{"2027", "2026-11-20", 0},
		{"2026-11-20", "2026-12-31", 0},
		{"", "2026-10-01", 1},
	}
	for i, a := range albums {
		id := fmt.Sprintf("l.%d", i)
		ref, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
			LibraryID: db.NullString(id), Name: id, ReleaseDate: a.date, CatalogReleaseDate: a.catalog, IsPrerelease: a.flagged, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "album", Ref: ref, Generation: 1}); err != nil {
			t.Fatal(err)
		}
		tref, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString("i." + id), Name: id, AlbumLibraryID: db.NullString(id), NoPlayParams: 1, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: tref, Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Time{}
	lib := library.New(d, nil)
	lib.SetClock(func() time.Time { return now })
	m := mapper{now: func() time.Time { return now }}
	var sawOut, sawNotOut bool
	for _, at := range []time.Time{
		localDay(t, "2026-11-01"), localDay(t, "2026-11-20").Add(-time.Second), localDay(t, "2026-11-20"),
		localDay(t, "2026-11-20").Add(12 * time.Hour), localDay(t, "2026-12-31"), localDay(t, "2027-01-01"),
	} {
		now = at
		for i := range albums {
			id := fmt.Sprintf("l.%d", i)
			a, _, err := lib.Album(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			song, err := lib.Song(ctx, "i."+id)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.libAlbum(a).Prerelease; song.Unreleased != got {
				t.Errorf("at %s, album %+v: the placeholder song is not out = %v, the album's countdown says %v", at.Format(time.DateTime), albums[i], song.Unreleased, got)
			}
			sawOut, sawNotOut = sawOut || !song.Unreleased, sawNotOut || song.Unreleased
		}
	}
	if !sawOut || !sawNotOut {
		t.Fatal("the cases hold only songs out or only songs not out: the comparison proves nothing")
	}
}

// Apple's "unknown" is the credit of a song it has not published, not an
// artist: a song with no playParams has none, whatever the source, while a
// song that plays keeps what it is credited to.
func TestUnknownIsNeverAnArtistOfAnUnpublishedSong(t *testing.T) {
	m := mapper{now: time.Now}
	for _, tc := range []struct {
		name         string
		artist       string
		noPlayParams bool
		want         string
	}{
		{"a placeholder", "unknown", true, ""},
		{"however Apple cases it", " Unknown ", true, ""},
		{"a song that plays by an artist called Unknown", "Unknown", false, "Unknown"},
		{"a real artist", "Mia Vale", true, "Mia Vale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.libSong(library.Song{LibraryID: "i.1", ArtistName: tc.artist, NoPlayParams: tc.noPlayParams}).Artist; got != tc.want {
				t.Errorf("library song artist = %q, want %q", got, tc.want)
			}
			play := `,"playParams":{"id":"1","kind":"song"}`
			if tc.noPlayParams {
				play = ""
			}
			a := decode[applemusic.Song](t, `{"id":"1","type":"songs","attributes":{"artistName":`+strconv.Quote(tc.artist)+play+`}}`)
			if got := m.amSong(a).Artist; got != tc.want {
				t.Errorf("catalog song artist = %q, want %q", got, tc.want)
			}
		})
	}
}

// The queue's items never carry the placeholder either.
func TestQueueItemsDropThePlaceholderArtist(t *testing.T) {
	it, _, err := trackItem(dbq.Track{LibraryID: db.NullString("i.1"), Name: "Track 3", ArtistName: "unknown", NoPlayParams: 1})
	if err != nil || it.Artist != "" {
		t.Errorf("stored track item artist = %q, %v", it.Artist, err)
	}
	a := decode[applemusic.Song](t, `{"id":"1","type":"songs","attributes":{"name":"Track 3","artistName":"unknown"}}`)
	if got := songItem(a).Artist; got != "" {
		t.Errorf("catalogue song item artist = %q", got)
	}
}
