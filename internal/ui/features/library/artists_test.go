package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func artistsAt(id string) string { return artistsPath + "?artist=" + id }

// artistsStub answers the artist queries itself: a library of its own
// artists, songs-only artists included.
type artistsStub struct {
	libraryAPI
	index    []services.ArtistEntry
	songs    map[string][]services.Song
	failSong atomic.Bool
	failAlb  atomic.Bool
	failIdx  atomic.Bool
	asked    atomic.Int32
}

func (s *artistsStub) ArtistIndex(context.Context) ([]services.ArtistEntry, error) {
	s.asked.Add(1)
	if s.failIdx.Load() {
		return nil, errors.New("index")
	}
	return s.index, nil
}

func (s *artistsStub) ArtistAlbums(_ context.Context, id string) (services.ArtistAlbums, error) {
	if s.failAlb.Load() {
		return services.ArtistAlbums{}, errors.New("albums")
	}
	return services.ArtistAlbums{Artist: services.Artist{ID: id, Name: id}}, nil
}

func (s *artistsStub) Artist(_ context.Context, id string) (services.ArtistDetail, error) {
	if s.failSong.Load() {
		return services.ArtistDetail{}, errors.New("songs")
	}
	return services.ArtistDetail{Artist: services.Artist{ID: id}, Songs: s.songs[id]}, nil
}

func artistsStubRig(t *testing.T, s *artistsStub, start string, w, h int) *pageRig {
	t.Helper()
	return newPageRig(t, start, w, h, func(l *library) { s.libraryAPI = l.lib; l.lib = s; l.auth = accountAPI{"signed_in"} })
}

// artistsFilter clicks the filter field, so the keys and the text go there.
func artistsFilter(t *testing.T, r *pageRig) {
	t.Helper()
	r.tt.Until("the filter and the list", r.tt.Shows("Filter artists", "Artists in your library"))
	if err := r.tt.Click("Filter artists"); err != nil {
		t.Fatal(err)
	}
	if !r.tt.Focused("Filter artists") {
		t.Fatal("a click on the filter did not focus it")
	}
}

func TestArtistsListAndDetailShow(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	r.tt.Until("the list and the first artist's albums", r.tt.Shows(
		"Artists", "12 artists", "Dara Finch", "Wren Atlas", "D", "H", "W",
		"Albums by Dara Finch in your library", "Singles and EPs by Dara Finch in your library",
		"Low Tide", "Cold Brew", "Albums", "Singles and EPs",
	))
	for _, w := range []string{
		"2 albums, 4 songs",          // the row's line
		"Artist · 2 albums, 4 songs", // the detail's
		"Play Dara Finch", "Shuffle", // the play control
		"2013", "2020 · Single", // release year, and the kind of a single
		"More for Dara Finch", // the artist's menu
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	// A page of another module is not linked while it does not exist.
	if r.tt.HasText("Full artist page") {
		t.Error("a link to a page no module registered")
	}
	// Nothing was written to the URL for the default.
	if got := r.router.Location(); got != artistsPath {
		t.Errorf("the URL is %q", got)
	}
}

func TestArtistsFullPageLinksOnlyToARegisteredPage(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, func(l *library) {
		l.env.HasRoute = func(p string) bool { return strings.HasPrefix(p, "/artist/") }
	})
	r.tt.Until("the button", r.tt.Shows("Full artist page"))
	if err := r.tt.Click("Full artist page"); err != nil {
		t.Fatal(err)
	}
	// Dara Finch's catalogue ID: the fixture gives artist n 500+n.
	if got := r.router.Location(); !strings.HasPrefix(got, "/artist/") {
		t.Errorf("went to %q", got)
	}
}

func TestArtistsTheURLOpensAnArtist(t *testing.T) {
	r := newPageRig(t, artistsAt("r.a2"), 1132, 900, nil)
	r.tt.Until("Mira Oyelaran", r.tt.Shows("Albums by Mira Oyelaran in your library", "Play Mira Oyelaran"))
	if r.tt.HasText("Albums by Dara Finch in your library") {
		t.Error("the first artist shows for a URL naming another")
	}

	// An artist the library does not hold, and an empty value: ignored, the
	// first artist stands in.
	for _, loc := range []string{artistsAt("r.nobody"), artistsPath + "?artist="} {
		r := newPageRig(t, loc, 1132, 900, nil)
		r.tt.Until("the first artist for "+loc, r.tt.Shows("Albums by Dara Finch in your library"))
	}
}

func TestArtistsFilterNarrowsAndSaysSo(t *testing.T) {
	r := newPageRig(t, artistsAt("r.a2"), 1132, 900, nil)
	artistsFilter(t, r)
	r.tt.Type("NOVA")
	r.tt.Until("one match", r.tt.Shows("1 of 12 artists", "Albums by Nova Ketter in your library"))
	if r.tt.HasText("Dara Finch") {
		t.Error("the filter keeps artists that do not match")
	}
	// Typing doesn't write the URL; choosing does.
	if got := r.router.Location(); got != artistsAt("r.a2") {
		t.Errorf("typing changed the URL to %q", got)
	}
	if got := r.tt.Announcements(); len(got) == 0 || got[len(got)-1] != "1 artist matches" {
		t.Errorf("announced %q", got)
	}

	// Without regard to accents.
	r.tt.Key(ui.Cmd, ui.KeyA)
	r.tt.Type("ines")
	r.tt.Until("Ines", r.tt.Shows("1 of 12 artists", "Albums by Ines Calder in your library"))

	r.tt.Key(ui.Cmd, ui.KeyA)
	r.tt.Type("zzz")
	r.tt.Until("no match", r.tt.Shows("No artist matches “zzz”."))
	if r.tt.HasText("Albums by") {
		t.Error("a detail shows for an empty result")
	}
	if got := r.tt.Announcements(); len(got) == 0 || got[len(got)-1] != "No artist matches “zzz”" {
		t.Errorf("announced %q", got)
	}
}

func TestArtistsKeyboardMovesTheOpenArtistAndReplacesTheURL(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	artistsFilter(t, r)
	key := func(k ui.Key) { r.tt.Key(0, k); r.tt.Drain() }

	key(ui.KeyDown)
	if got := r.router.Location(); got != artistsAt("r.a3") {
		t.Fatalf("Down: %q", got)
	}
	r.tt.Until("Hollis & Vane", r.tt.Shows("Albums by Hollis & Vane in your library"))
	key(ui.KeyDown)
	key(ui.KeyUp)
	if got := r.router.Location(); got != artistsAt("r.a3") {
		t.Errorf("Down, Up: %q", got)
	}
	// Page steps move by ten, and stop at the ends.
	key(ui.KeyPageDown)
	if got := r.router.Location(); got != artistsAt("r.a9") {
		t.Errorf("PageDown: %q", got)
	}
	key(ui.KeyDown)
	if got := r.router.Location(); got != artistsAt("r.a9") {
		t.Errorf("Down at the end: %q", got)
	}
	key(ui.KeyPageUp) // from the last of twelve: the second
	if got := r.router.Location(); got != artistsAt("r.a3") {
		t.Errorf("PageUp: %q", got)
	}
	key(ui.KeyPageUp)
	if got := r.router.Location(); got != artistsAt("r.a10") {
		t.Errorf("PageUp at the start: %q", got)
	}
	// The history has one entry: walking replaced it.
	r.router.Back()
	if got := r.router.Location(); got != artistsAt("r.a10") {
		t.Errorf("Back left the page: %q", got)
	}
}

func TestArtistsListKeysHomeEndEnterAndTyping(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	r.tt.Until("the list", r.tt.Shows("Wren Atlas", "Albums by Dara Finch in your library"))
	// A click puts the keys on the list.
	if err := r.tt.Click("Mira Oyelaran"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != artistsAt("r.a2") {
		t.Fatalf("a click opened %q", got)
	}
	r.tt.Until("Mira", r.tt.Shows("Albums by Mira Oyelaran in your library"))
	if !r.tt.Focused("Artists in your library") {
		t.Fatal("the list lost the keys after a click")
	}
	key := func(k ui.Key) { r.tt.Key(0, k); r.tt.Drain() }
	key(ui.KeyEnd)
	if got := r.router.Location(); got != artistsAt("r.a9") {
		t.Errorf("End: %q", got)
	}
	key(ui.KeyHome)
	if got := r.router.Location(); got != artistsAt("r.a10") {
		t.Errorf("Home: %q", got)
	}

	// A letter typed on the list goes to the filter, which takes the focus.
	r.tt.Type("wre")
	r.tt.Until("the filter has it", func() bool { return r.tt.Focused("Filter artists") && r.tt.HasText("1 of 12 artists") })
	r.tt.Until("Wren", r.tt.Shows("Albums by Wren Atlas in your library"))

	// Enter in the field opens the top match in the URL and goes into the detail.
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("in the URL", func() bool { return r.router.Location() == artistsAt("r.a9") })
	r.tt.Until("the detail has the focus", func() bool { return !r.tt.Focused("Filter artists") })

	// Escape in the detail returns to the filter, then clears it.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Until("the filter again", func() bool { return r.tt.Focused("Filter artists") })
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Until("cleared", r.tt.Shows("12 artists"))
	if r.tt.HasText("1 of 12 artists") {
		t.Error("Escape did not clear the filter")
	}
}

func TestArtistsRightArrowGoesIntoTheDetailOnlyFromTheEndOfTheText(t *testing.T) {
	r := newPageRig(t, artistsAt("r.a2"), 1132, 900, nil)
	artistsFilter(t, r)
	r.tt.Type("kof")
	r.tt.Until("Kofi", r.tt.Shows("Albums by Kofi Bellamy in your library"))
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Key(0, ui.KeyRight) // the caret moves back to the end
	r.tt.Drain()
	if got := r.router.Location(); got != artistsAt("r.a2") {
		t.Errorf("→ inside the text opened %q", got)
	}
	r.tt.Key(0, ui.KeyRight) // at the end
	r.tt.Until("in the URL", func() bool { return r.router.Location() == artistsAt("r.a11") })
}

func TestArtistsClickingOpensAnEntryOfTheHistory(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	r.tt.Until("the list", r.tt.Shows("Mira Oyelaran"))
	if err := r.tt.Click("Mira Oyelaran"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != artistsAt("r.a2") {
		t.Fatalf("URL %q", got)
	}
	r.tt.Until("Mira", r.tt.Shows("Albums by Mira Oyelaran in your library"))
	r.router.Back()
	if got := r.router.Location(); got != artistsPath {
		t.Errorf("Back went to %q", got)
	}
}

func TestArtistsNarrowWindowTakesTurns(t *testing.T) {
	r := newPageRig(t, artistsPath, 700, 900, nil)
	r.tt.Until("the list alone", r.tt.Shows("Dara Finch", "12 artists"))
	r.tt.Drain()
	if r.tt.HasText("Albums by Dara Finch in your library") {
		t.Fatal("a narrow window shows the detail beside the list")
	}
	// Arrows move a cursor; Enter opens it as a new history entry.
	artistsFilter(t, r)
	r.tt.Key(0, ui.KeyDown)
	r.tt.Key(0, ui.KeyDown)
	r.tt.Drain()
	if got := r.router.Location(); got != artistsPath {
		t.Fatalf("a cursor wrote the URL: %q", got)
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("Hollis & Vane alone", r.tt.Shows("Albums by Hollis & Vane in your library"))
	if got := r.router.Location(); got != artistsAt("r.a3") {
		t.Errorf("Enter opened %q", got)
	}
	// The list is out of the way; the way back is focused.
	if r.tt.HasText("12 artists") {
		t.Error("the list shows beside the artist")
	}
	r.tt.Until("Back has the focus", func() bool { return r.tt.Focused("Artists") })
	// Back returns to the list (the artist was opened from it: a step back in
	// the history).
	if err := r.tt.Click("Artists"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the list again", r.tt.Shows("12 artists"))
	if got := r.router.Location(); got != artistsPath {
		t.Errorf("Back went to %q", got)
	}
	r.tt.Until("the filter has the focus", func() bool { return r.tt.Focused("Filter artists") })

	// A reload with an artist: Back leaves the entry.
	r2 := newPageRig(t, artistsAt("r.a8"), 700, 900, nil)
	r2.tt.Until("Ines alone", r2.tt.Shows("Albums by Ines Calder in your library"))
	if err := r2.tt.Click("Artists"); err != nil {
		t.Fatal(err)
	}
	r2.tt.Until("the list", r2.tt.Shows("12 artists"))
	if got := r2.router.Location(); got != artistsPath {
		t.Errorf("Back from a reload went to %q", got)
	}

	// An unknown artist falls back to the list.
	r3 := newPageRig(t, artistsAt("r.nobody"), 700, 900, nil)
	r3.tt.Until("the list", r3.tt.Shows("12 artists"))
	if r3.tt.HasText("Albums by") {
		t.Error("a detail for an unknown artist")
	}
}

func TestArtistsPlayAndShufflePlayTheArtistsSongs(t *testing.T) {
	r := newPageRig(t, artistsAt("r.a10"), 1132, 900, nil)
	r.tt.Until("the buttons", r.tt.Shows("Play Dara Finch", "Shuffle"))
	if err := r.tt.Click("Play Dara Finch"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the engine to play", func() bool { return len(r.h.Engine.Played()) > 0 })
	pb := services.Service[services.PlaybackService](r.h.Core)
	st := pb.State()
	if st.Container == nil || st.Container.Kind != "artist" || st.Container.ID != "r.a10" {
		t.Errorf("the queue's container is %+v, want artist r.a10", st.Container)
	}
	// Dara Finch holds two albums of two songs each.
	if n := len(pb.Queue().Items); n != 4 {
		t.Errorf("queue of %d songs, want 4", n)
	}

	r2 := newPageRig(t, artistsAt("r.a10"), 1132, 900, nil)
	r2.tt.Until("the buttons", r2.tt.Shows("Shuffle"))
	if err := r2.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	r2.tt.Until("the engine to play", func() bool { return len(r2.h.Engine.Played()) > 0 })
	if st := services.Service[services.PlaybackService](r2.h.Core).State(); st.Container == nil || st.Container.ID != "r.a10" {
		t.Errorf("shuffle's container is %+v", st.Container)
	}
}

func artistsSolo() *artistsStub {
	song := func(id, name string) services.Song {
		return services.Song{ID: id, CatalogID: id, Name: name, Playable: true, Artist: "Solo Singer"}
	}
	return &artistsStub{
		index: []services.ArtistEntry{
			{ID: "r.solo", Name: "Solo Singer", Songs: 2},
			{ID: "r.ghost", Name: "Ghost", Albums: 0, Songs: 0},
		},
		songs: map[string][]services.Song{"r.solo": {song("s1", "Quiet Song"), song("s2", "Louder Song")}},
	}
}

func TestArtistsWithSongsOnlyShowTheirSongs(t *testing.T) {
	r := artistsStubRig(t, artistsSolo(), artistsAt("r.solo"), 1132, 900)
	r.tt.Until("the songs", r.tt.Shows("Songs in your library", "Quiet Song", "Louder Song", "Artist · 2 songs"))
	if r.tt.HasText("Singles and EPs") || r.tt.HasText("Albums by") {
		t.Error("album sections for an artist without albums")
	}
}

func TestArtistsWithNothingInTheLibrarySaySo(t *testing.T) {
	r := artistsStubRig(t, artistsSolo(), artistsAt("r.ghost"), 1132, 900)
	r.tt.Until("the line", r.tt.Shows("Nothing by Ghost is in your library yet.", "Artist"))
	if _, ok := r.tt.Find("Songs in your library"); ok {
		t.Error("a songs list for no songs")
	}
}

func TestArtistsDetailErrorsRetry(t *testing.T) {
	s := artistsSolo()
	s.failAlb.Store(true)
	r := artistsStubRig(t, s, artistsAt("r.solo"), 1132, 900)
	r.tt.Until("the albums error", r.tt.Shows("Couldn't read this artist's albums from this computer", "Try again"))
	s.failAlb.Store(false)
	s.failSong.Store(true)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the songs error", r.tt.Shows("Couldn't read this artist's songs from this computer", "Try again"))
	s.failSong.Store(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the songs", r.tt.Shows("Quiet Song"))
}

// artistsSignedIn is cond, once the rig's account is the signed-in one: the
// harness's own account restore may announce "signed out" after the rig
// replaced the service, which the page would believe.
func artistsSignedIn(r *pageRig, cond func() bool) func() bool {
	return func() bool {
		if r.tt.HasText("Sign in to see your artists") {
			r.h.Env.Bus.Emit(services.EventAuthChanged, services.AuthStatus{State: "signed_in"})
		}
		return cond()
	}
}

func TestArtistsGateSignedOut(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 800, nil, uitest.Empty())
	r.tt.Until("signed out", r.tt.Shows("Artists", "Sign in to see your artists"))
	if r.tt.HasText("Filter artists") {
		t.Error("the filter shows for a library that cannot be read")
	}
}

func TestArtistsGateSyncing(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 800, func(l *library) { l.auth = accountAPI{"signed_in"} }, uitest.Empty())
	shows := r.tt.Shows("Artists", "Your library is syncing",
		"Your artists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.")
	r.tt.Until("syncing", artistsSignedIn(r, shows))
}

func TestArtistsGateEmptyOnceSynced(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 800, func(l *library) { l.auth = accountAPI{"signed_in"} })
	emptyTheSyncedLibrary(r)
	r.tt.Until("empty", artistsSignedIn(r, r.tt.Shows("Artists", "No artists in your library yet",
		"Every artist with a song or album in your library is listed here, A to Z.")))
}

func TestArtistsGateShowsAReadErrorAndRetries(t *testing.T) {
	s := artistsSolo()
	s.failIdx.Store(true)
	r := artistsStubRig(t, s, artistsPath, 1132, 800)
	r.tt.Until("the error", r.tt.Shows("Artists", "Couldn't read your artists from this computer", "Try again"))
	s.failIdx.Store(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the artists", r.tt.Shows("Solo Singer", "2 artists"))
}

func TestArtistsASyncRefreshesTheList(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	r.tt.Until("the list", r.tt.Shows("12 artists"))
	res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO artists (library_id, catalog_id, name, updated_at) VALUES ('r.new', '999', 'Aaron Newcomer', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, generation) VALUES ('artist', ?, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "artists", Done: 1, Total: 1, Finished: true})
	r.tt.Until("the new artist", r.tt.Shows("Aaron Newcomer", "13 artists"))
}

// A long library builds only the rows in view; End reaches the last artist.
func TestArtistsLongListIsVirtualised(t *testing.T) {
	s := &artistsStub{}
	for i := range 600 {
		s.index = append(s.index, services.ArtistEntry{ID: fmt.Sprintf("r.%03d", i), Name: fmt.Sprintf("Band %03d", i), Albums: 1, Songs: 2})
	}
	r := artistsStubRig(t, s, artistsPath, 1132, 700)
	r.tt.Until("the first rows", r.tt.Shows("600 artists", "Band 000", "Band 010"))
	if r.tt.HasText("Band 300") || r.tt.HasText("Band 599") {
		t.Fatal("the list builds rows far out of view")
	}
	if err := r.tt.Click("Band 002"); err != nil {
		t.Fatal(err)
	}
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Until("the last artist", r.tt.Shows("Band 599"))
	if r.tt.HasText("Band 000") {
		t.Error("the first rows stay built at the end of the list")
	}
	if got := r.router.Location(); got != artistsAt("r.599") {
		t.Errorf("End: %q", got)
	}
	r.tt.Key(0, ui.KeyHome)
	r.tt.Until("the first artist", r.tt.Shows("Band 000"))
	// Wheel scrolling builds the rows it reaches.
	r.tt.Scroll(100, 400, 0, 6000)
	r.tt.Until("rows far down", func() bool { return !r.tt.HasText("Band 010") && r.tt.HasText("Band 128") })
}
