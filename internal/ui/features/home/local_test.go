package home

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

// emptyHome is a signed-in Home with no shelves: the local ones show.
func emptyHome() rigOpt {
	return withHome(services.HomePage{Sections: []services.Section{}, FetchedAt: 1})
}

// signedOut is an account with no sign-in: the local shelves alone.
func signedOut() rigOpt { return withAuth(services.AuthStatus{State: "signed_out"}) }

func heardSongs(n int, prefix string) []services.Song {
	var out []services.Song
	for i := 1; i <= n; i++ {
		out = append(out, services.Song{
			ID: fmt.Sprintf("%s%d", prefix, i), Name: fmt.Sprintf("%s %d", prefix, i), Artist: "Heard Artist",
			Album: "Heard Album", AlbumID: "al1", ArtistID: "ar1", Playable: true, DurationMs: 180000,
		})
	}
	return out
}

func TestRecentlyPlayedListsAppleSongsAndPlaysFromTheRow(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) { l.recent = heardSongs(3, "Heard") }))
	r.until("the songs", "Recently played", "Heard 1", "Heard 3")
	requireTexts(t, r, "Heard Artist", "Heard Album")
	requireNoTexts(t, r, "Played on this computer")
	if _, ok := r.tt.Find("Recently played songs"); !ok {
		t.Error("the table has no name")
	}
	if n := r.lib.count("recent 10 0"); n != 1 {
		t.Errorf("RecentlyPlayed(10, 0) asked %d times", n)
	}
	// Enter on a row plays the list from it, labelled "recently-played".
	title, _ := r.tt.Find("Heard 2")
	r.tt.ClickAt(title.X+title.W/6, title.Y+title.H/2) // on the title, not the artist or album links
	r.tt.Frame()
	r.tt.Frame()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if got := r.rec.all(); !slices.Equal(got, []string{"songs:recently-played@1/3"}) {
		t.Errorf("played %q", got)
	}
}

func TestRecentlyPlayedShowsLoadingRowsWhileItReads(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) { l.recentGate = gate }))
	r.until("the shelf", "Recently played")
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Errorf("no loading rows; texts %q", r.tt.Texts())
	}
	requireNoTexts(t, r, "Nothing played yet")
	close(gate)
	r.until("the empty state", "Nothing played yet")
}

func TestRecentlyPlayedFallsBackToThisComputersHistory(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) {
		l.recentErr = errDown
		l.days = []services.HistoryDay{
			{Day: 2, Entries: entries(heardSongs(6, "Newer"))},
			{Day: 1, Entries: entries(heardSongs(6, "Older"))},
		}
	}))
	r.until("this computer's songs", "Played on this computer", "Newer 1")
	// Ten of them, newest first, from the last 30 days.
	requireTexts(t, r, "Newer 6", "Older 4")
	requireNoTexts(t, r, "Older 5", "Recently played")
	if n := r.lib.count("history 30"); n != 1 {
		t.Errorf("History(30) asked %d times", n)
	}
}

func entries(songs []services.Song) []services.HistoryEntry {
	out := make([]services.HistoryEntry, len(songs))
	for i, s := range songs {
		out[i] = services.HistoryEntry{Song: s}
	}
	return out
}

func TestRecentlyPlayedEmptyStatesNameTheirSource(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) { l.recent = []services.Song{} }))
	r.until("the empty state", "Nothing played yet", "Songs you play in Sona or Apple Music on any device appear here.")
	requireTexts(t, r, "Recently played")

	// With Apple unreachable and nothing played here either.
	off := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) { l.recentErr = errDown }))
	off.until("the offline empty state", "Played on this computer", "Nothing played yet", "Songs you play in Sona appear here.")
	requireNoTexts(t, off, "Songs you play in Sona or Apple Music on any device appear here.")
}

func TestRecentlyPlayedErrorWithNoFallbackRetries(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, emptyHome(), withLib(func(l *fakeLib) { l.recentErr, l.historyErr = errDown, errDown }))
	r.until("the error", "Recently played", "Couldn't load what you played recently", "Try again")
	r.lib.mu.Lock()
	r.lib.recentErr, r.lib.recent = nil, heardSongs(2, "Back")
	r.lib.mu.Unlock()
	r.click("Try again")
	r.until("the songs", "Back 1", "Back 2")
	requireNoTexts(t, r, "Couldn't load what you played recently")
}

func TestRecentlyAddedListsTheNewestTwelveAlbumsAndLinksToTheLibrary(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, signedOut(), withLib(func(l *fakeLib) {
		for i := 1; i <= 14; i++ {
			l.albums = append(l.albums, services.Album{ID: fmt.Sprintf("a%d", i), Name: fmt.Sprintf("Added %d", i), Artist: "Adder"})
		}
	}))
	r.until("the shelf", "Recently added", "Added 1")
	if n := r.lib.count("albums dateAdded true \"\" 0 12"); n != 1 {
		t.Errorf("Albums asked %d times with the shelf's query; asked %q", n, r.lib.asked)
	}
	requireTexts(t, r, "Albums recently added to your library", "Adder")
	if _, ok := r.tt.Find("Albums recently added to your library"); !ok {
		t.Error("the row has no name")
	}
	// A tile opens the album; See all the library.
	r.click("Added 2")
	if got := r.path(); got != "/album/a2" {
		t.Errorf("the tile opened %q", got)
	}
}

func TestRecentlyAddedIsHiddenWhenEmptyAndShowsItsErrorWithRetry(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, signedOut(), withLib(func(l *fakeLib) {
		l.playlists = []services.Playlist{{ID: "p1", Name: "Only Playlist", TrackCount: 1}}
	}))
	r.until("the playlists", "Your playlists", "Only Playlist", "1 song")
	requireNoTexts(t, r, "Recently added")

	e := newRig(t, "/", 1132, 1253, signedOut(), withLib(func(l *fakeLib) { l.albumsErr = errDown }))
	e.until("the error", "Couldn't read your albums", "Try again")
	e.lib.mu.Lock()
	e.lib.albumsErr, e.lib.albums = nil, []services.Album{{ID: "a1", Name: "Found Album"}}
	e.lib.mu.Unlock()
	e.click("Try again")
	e.until("the albums", "Found Album")
	requireNoTexts(t, e, "Couldn't read your albums")
}

func TestYourPlaylistsAreHiddenOnAnErrorAndLinkToTheLibrary(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, signedOut(), withLib(func(l *fakeLib) {
		l.playlistErr = errDown
		l.albums = []services.Album{{ID: "a1", Name: "Found Album"}}
	}))
	r.until("the albums", "Found Album")
	r.tt.Drain()
	requireNoTexts(t, r, "Your playlists", "Couldn't read your playlists")

	p := newRig(t, "/", 1132, 1253, signedOut(), withLib(func(l *fakeLib) {
		l.playlists = []services.Playlist{{ID: "p1", Name: "Mix One", TrackCount: 3}, {ID: "p2", Name: "Mix Two", Curator: "Some Curator"}}
	}))
	p.until("the playlists", "Your playlists", "Mix One", "3 songs", "Mix Two", "Some Curator")
	if _, ok := p.tt.Find("Recently updated playlists"); !ok {
		t.Error("the row has no name")
	}
	p.click("See all")
	if got := p.path(); got != "/library/playlists" {
		t.Errorf("See all went to %q", got)
	}
}
