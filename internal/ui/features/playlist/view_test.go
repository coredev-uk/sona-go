package playlist

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// viewSongs are songs in an order that is neither alphabetical by any column
// nor by length: the playlist's own, which the sort leaves and returns to.
func viewSongs() []services.Song {
	mk := func(id, name, artist, album string, ms int64) services.Song {
		s := pgSong(id, name, artist, album)
		s.DurationMs = ms
		return s
	}
	return []services.Song{
		mk("a", "Charlie", "Zed", "Low Tide", 180_000),
		mk("b", "alpha", "Mia", "Salt Flats", 240_000),
		mk("c", "Bravo", "Al", "Lantern", 200_000),
		mk("d", "Échelle", "mia", "Salt Flats", 200_000),
	}
}

var viewNames = []string{"Charlie", "alpha", "Bravo", "Échelle"}

func TestViewOrder(t *testing.T) {
	songs := viewSongs()
	for _, tc := range []struct {
		name  string
		key   string
		desc  bool
		query string
		want  []int
	}{
		{"the playlist's order is the default", "", false, "", []int{0, 1, 2, 3}},
		{"a descending default is still the playlist's order", "", true, "", []int{0, 1, 2, 3}},
		{"an unknown key is the playlist's order", "year", false, "", []int{0, 1, 2, 3}},
		{"title folds case and accents", kit.SortTitle, false, "", []int{1, 2, 0, 3}},
		{"title descending", kit.SortTitle, true, "", []int{3, 0, 2, 1}},
		{"artist folds case: Mia and mia tie and keep the playlist's order", kit.SortArtist, false, "", []int{2, 1, 3, 0}},
		{"artist descending keeps ties in the playlist's order", kit.SortArtist, true, "", []int{0, 1, 3, 2}},
		{"album", kit.SortAlbum, false, "", []int{2, 0, 1, 3}},
		{"duration", kit.SortDuration, false, "", []int{0, 2, 3, 1}},
		{"duration descending keeps ties in the playlist's order", kit.SortDuration, true, "", []int{1, 2, 3, 0}},
		{"an empty query keeps every song", "", false, "", []int{0, 1, 2, 3}},
		{"a blank query keeps every song", "", false, "  ", []int{0, 1, 2, 3}},
		{"the query matches the name", "", false, "bravo", []int{2}},
		{"the query matches the artist", "", false, "zed", []int{0}},
		{"the query matches the album", "", false, "lantern", []int{2}},
		{"the query folds case and accents both ways", "", false, "ECHELLE", []int{3}},
		{"an accented query finds an unaccented name", "", false, "álpha", []int{1}},
		{"the query is part of a word", "", false, "rav", []int{2}},
		{"every word of the query must match somewhere", "", false, "mia salt", []int{1, 3}},
		{"words may come from different fields", "", false, "mia alpha", []int{1}},
		{"no match is none", "", false, "zzz", []int{}},
		{"the filtered songs are sorted, as original indices", kit.SortTitle, true, "salt", []int{3, 1}},
		{"filtered and by duration", kit.SortDuration, false, "e", []int{0, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := viewOrder(songs, tc.key, tc.desc, tc.query)
			if got == nil || !slices.Equal(got, tc.want) {
				t.Errorf("viewOrder(%q, desc %v, %q) = %v, want %v", tc.key, tc.desc, tc.query, got, tc.want)
			}
		})
	}
}

// A song added twice is two entries with one ID: each keeps its own place.
func TestViewOrderKeepsEachCopyOfASongApart(t *testing.T) {
	songs := []services.Song{song("a", "Same", "x"), song("b", "Other", "x"), song("a", "Same", "x")}
	if got := viewOrder(songs, "", false, "same"); !slices.Equal(got, []int{0, 2}) {
		t.Errorf("found: %v", got)
	}
	if got := viewOrder(songs, kit.SortTitle, true, ""); !slices.Equal(got, []int{0, 2, 1}) {
		t.Errorf("by title, descending: %v", got)
	}
	if got := viewOrder(songs, kit.SortTitle, false, ""); !slices.Equal(got, []int{1, 0, 2}) {
		t.Errorf("by title: %v", got)
	}
}

func TestViewOrderReadsDigitsAsNumbers(t *testing.T) {
	songs := []services.Song{song("1", "Track 10", "x"), song("2", "Track 2", "x"), song("3", "Track 1", "x")}
	if got := viewOrder(songs, kit.SortTitle, false, ""); !slices.Equal(got, []int{2, 1, 0}) {
		t.Errorf("order %v, want Track 1, 2, 10", got)
	}
}

func TestViewOrderDoesNotTouchTheTracksAndHandlesNone(t *testing.T) {
	songs := viewSongs()
	want := slices.Clone(songs)
	viewOrder(songs, kit.SortTitle, true, "a")
	if !slices.Equal(songs, want) {
		t.Error("viewOrder changed the tracks")
	}
	if got := viewOrder(nil, kit.SortTitle, false, "x"); len(got) != 0 {
		t.Errorf("no tracks: %v", got)
	}
}

func names(songs []services.Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.Name
	}
	return out
}

// The page asks for the view every frame: it is worked out again only when
// the sort, the query or the tracks change.
func TestPageViewIsMemoised(t *testing.T) {
	var pg page
	songs := viewSongs()
	first := pg.view(songs)
	if len(first.songs) != 4 || first.order != nil || &first.songs[0] != &songs[0] {
		t.Fatalf("with no sort and no query the page lists the tracks themselves: %+v", first)
	}

	pg.sort.Key = kit.SortTitle
	sorted := pg.view(songs)
	if got := names(sorted.songs); !slices.Equal(got, []string{"alpha", "Bravo", "Charlie", "Échelle"}) || !slices.Equal(sorted.order, []int{1, 2, 0, 3}) {
		t.Fatalf("sorted: %v %v", got, sorted.order)
	}
	if again := pg.view(songs); &again.songs[0] != &sorted.songs[0] || &again.order[0] != &sorted.order[0] {
		t.Error("the view was worked out again for nothing")
	}

	pg.query = "salt"
	narrowed := pg.view(songs)
	if got := names(narrowed.songs); !slices.Equal(got, []string{"alpha", "Échelle"}) {
		t.Errorf("narrowed: %v", got)
	}
	if narrowed.place(1) != 3 {
		t.Errorf("place(1) = %d, want the playlist's 3", narrowed.place(1))
	}

	// Other tracks (an edit made a new list) are listed afresh.
	if got := names(pg.view(songs[:3]).songs); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("after the tracks changed: %v", got)
	}
}

func TestPageEnterResetsTheViewPerPlaylist(t *testing.T) {
	var pg page
	pg.enter("p.1")
	pg.sort.Key, pg.sort.Desc, pg.query = kit.SortArtist, true, "salt"
	pg.enter("p.1")
	if pg.sort.Key != kit.SortArtist || !pg.sort.Desc || pg.query != "salt" {
		t.Errorf("the same playlist lost its view: %+v %q", pg.sort, pg.query)
	}
	pg.enter("p.2")
	if pg.sort.Key != "" || pg.sort.Desc || pg.query != "" {
		t.Errorf("another playlist kept the view: %+v %q", pg.sort, pg.query)
	}
}

// viewRig is a playlist page of viewSongs (a library playlist: editable).
func viewRig(t *testing.T) *pageRig {
	t.Helper()
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", viewSongs()...))
	r.shows("Charlie", "Find in playlist")
	return r
}

// find focuses the page's field and types text into it.
func (r *pageRig) find(text string) {
	r.t.Helper()
	r.tt.Settle()
	if err := r.tt.Click("Find in playlist"); err != nil {
		r.t.Fatal(err)
	}
	if !r.tt.Focused("Find in playlist") {
		r.t.Fatal("a click on the field did not focus it")
	}
	r.tt.Type(text)
	r.tt.Frame()
}

// sortOfHeader is the sort state of the column header named title: "" for
// none, else "ascending" or "descending".
func sortOfHeader(t *testing.T, tr access.Tree, title string) string {
	t.Helper()
	h, ok := tr.Find(ui.RoleColumnHeader, title)
	if !ok {
		t.Fatalf("no column header %q:\n%s", title, tr.Dump())
	}
	switch {
	case h.Has(access.SortAscending):
		return "ascending"
	case h.Has(access.SortDescending):
		return "descending"
	}
	return ""
}

func TestPageHeaderSortsAndAThirdClickGoesBackToThePlaylistOrder(t *testing.T) {
	r := viewRig(t)
	if got := r.order(viewNames...); !slices.Equal(got, viewNames) {
		t.Fatalf("the playlist's order at first: %v", got)
	}

	for i, want := range [][]string{
		{"alpha", "Bravo", "Charlie", "Échelle"}, // ascending
		{"Échelle", "Charlie", "Bravo", "alpha"}, // descending
		viewNames, // the third click: the playlist's order
	} {
		r.click("Title")
		r.tt.Settle()
		if got := r.order(viewNames...); !slices.Equal(got, want) {
			t.Errorf("after click %d on Title: %v, want %v", i+1, got, want)
		}
	}

	// Another column starts from ascending, from wherever the last one was.
	r.click("Duration")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"Charlie", "Bravo", "Échelle", "alpha"}) {
		t.Errorf("by duration: %v", got)
	}
	r.click("Duration")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"alpha", "Bravo", "Échelle", "Charlie"}) {
		t.Errorf("by duration, longest first (ties in playlist order): %v", got)
	}
	r.click("Artist")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"Bravo", "alpha", "Échelle", "Charlie"}) {
		t.Errorf("by artist: %v", got)
	}
	r.click("Album")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"Bravo", "Charlie", "alpha", "Échelle"}) {
		t.Errorf("by album: %v", got)
	}
}

func TestPageSortIsAnnounced(t *testing.T) {
	r := viewRig(t)
	r.tt.Announcements()
	for _, want := range []string{
		"Songs sorted by Duration, ascending", "Songs sorted by Duration, descending", "Songs in playlist order",
	} {
		r.click("Duration")
		if got := r.tt.Announcements(); !slices.Contains(got, want) {
			t.Errorf("announcements %q, want %q", got, want)
		}
	}
}

func TestPageFilterCountsAndEscapeClears(t *testing.T) {
	r := viewRig(t)
	if r.tt.HasText("of 4 songs") {
		t.Error("a count with no query")
	}
	r.tt.Announcements()
	r.find("salt")
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	if got := r.order(viewNames...); !slices.Equal(got, []string{"alpha", "Échelle"}) {
		t.Errorf("the songs found: %v", got)
	}
	got := r.tt.Announcements()
	if !slices.Contains(got, "2 songs match") {
		t.Errorf("announcements %q", got)
	}
	// Each keystroke is announced with its own count, never the last one's.
	for _, stale := range []string{"4 songs match", "3 songs match"} {
		if slices.Contains(got, stale) {
			t.Errorf("announcements %q hold a count of an earlier query", got)
		}
	}

	r.tt.Type("x")
	r.tt.Frame()
	r.shows("No song in this playlist matches “saltx”.", "0 of 4 songs")
	if got := r.order(viewNames...); len(got) != 0 {
		t.Errorf("songs listed with no match: %v", got)
	}
	if got := r.tt.Announcements(); !slices.Contains(got, "No song matches “saltx”") {
		t.Errorf("announcements %q", got)
	}
	if r.tt.HasText("Songs in Road Trip") {
		t.Error("an empty table is still there")
	}

	r.tt.Key(0, ui.KeyEscape)
	r.tt.Frame()
	r.shows("Songs in Road Trip")
	if r.tt.HasText("of 4 songs") || r.tt.HasText("No song in this playlist matches “saltx”.") {
		t.Errorf("Escape did not clear the query: %q", r.tt.Texts())
	}
	if got := r.order(viewNames...); !slices.Equal(got, viewNames) {
		t.Errorf("after Escape: %v", got)
	}
	if !r.tt.Focused("Find in playlist") {
		t.Error("Escape took the focus out of the field")
	}

	// A query and a sort together: the songs found, in the sort's order.
	r.tt.Type("salt")
	r.click("Title")
	r.click("Title")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"Échelle", "alpha"}) {
		t.Errorf("found and sorted: %v", got)
	}
}

func TestPagePlayAndRowsPlayTheDisplayedOrder(t *testing.T) {
	r := viewRig(t)
	r.click("Title") // alpha, Bravo, Charlie, Échelle: b c a d
	r.tt.Settle()
	r.click("Play Road Trip")
	r.tt.Until("the play", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got != "[b c a d] start=0 from=playlist:p.1" {
		t.Errorf("Play: %q", got)
	}

	// A row plays from where it is shown, with the shown songs as the queue.
	r.click("Charlie")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the row play", func() bool { return len(r.play.played()) == 2 })
	if got := r.play.played()[1]; got != "[b c a d] start=2 from=playlist:p.1" {
		t.Errorf("row play: %q", got)
	}

	// With a query the queue is the songs found.
	r.find("salt")
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	r.click("Shuffle")
	r.tt.Until("the shuffle", func() bool { return len(r.play.played()) == 3 })
	if got := r.play.played()[2]; !strings.HasPrefix(got, "[b d] start=") {
		t.Errorf("shuffle: %q", got)
	}
	r.click("Play Road Trip")
	r.tt.Until("the play", func() bool { return len(r.play.played()) == 4 })
	if got := r.play.played()[3]; got != "[b d] start=0 from=playlist:p.1" {
		t.Errorf("Play of the songs found: %q", got)
	}
}

// removeFromMenu opens the row menu of the song listed as name and removes it
// from the playlist.
func removeFromMenu(t *testing.T, r *pageRig, name string) {
	t.Helper()
	if err := r.tt.RightClick(name); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the row menu", r.tt.Shows("Remove from playlist"))
	r.click("Remove from playlist")
}

func TestPageRemoveSendsTheOriginalPosition(t *testing.T) {
	r := viewRig(t)
	r.click("Title") // alpha, Bravo, Charlie, Échelle
	r.tt.Settle()
	// Charlie is the third row as listed and the first in the playlist.
	removeFromMenu(t, r, "Charlie")
	r.did("RemoveTracks p.1 [0] [a]")
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
}

func TestPageRemoveOfAFoundSongSendsTheOriginalPosition(t *testing.T) {
	r := viewRig(t)
	r.find("salt") // alpha, Échelle: positions 1 and 3
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	removeFromMenu(t, r, "Échelle")
	r.did("RemoveTracks p.1 [3] [d]")
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
	// The song is gone from the playlist and from the songs found.
	r.shows("1 of 3 songs")
	if got := r.order(viewNames...); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("after the removal: %v", got)
	}
}

func TestPageBulkRemoveSendsTheOriginalPositions(t *testing.T) {
	r := viewRig(t)
	r.click("Title") // alpha, Bravo, Charlie, Échelle: positions 1 2 0 3
	r.tt.Settle()
	r.click("Bravo")
	r.tt.Key(ui.Shift, ui.KeyDown) // Bravo and Charlie
	r.tt.Key(ui.Shift, ui.KeyDown) // and Échelle
	r.tt.Frame()
	if err := r.tt.RightClick("Charlie"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the bulk menu", r.tt.Shows("3 songs selected", "Remove from playlist"))
	r.click("Remove from playlist")
	r.did("RemoveTracks p.1 [2 0 3] [c a d]")
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
}

// Removing the second copy of a song listed twice sends the second copy's
// position, with a query narrowing the list to both copies.
func TestPageRemoveOfTheSecondCopyOfADuplicateSendsItsPosition(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip",
		song("a", "Same", "x"), song("b", "Other", "x"), song("a", "Same", "x")))
	r.shows("Same", "Find in playlist")
	r.find("same")
	r.tt.Until("the count", r.tt.Shows("2 of 3 songs"))
	r.click("Same") // the first copy: focus the list
	r.tt.Key(0, ui.KeyDown)
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Until("the row menu", r.tt.Shows("Remove from playlist"))
	r.click("Remove from playlist")
	r.did("RemoveTracks p.1 [2] [a]")
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
}

// A list that got shorter keeps a row that is a Tab stop: the row focused
// before the filter may no longer be there.
func TestPageFilterKeepsARowInTheTabOrder(t *testing.T) {
	r := viewRig(t)
	r.click("Bravo") // the third row has the list's focus
	r.find("salt")   // two songs: alpha and Échelle
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	var stop string
	for range 8 { // the clear button is no stop; then the four sort buttons, then the rows
		r.tt.Key(0, ui.KeyTab)
		r.tt.Frame()
		for _, name := range []string{"alpha", "Échelle"} {
			if r.tt.Focused(name) {
				stop = name
			}
		}
		if stop != "" {
			break
		}
	}
	if stop == "" {
		t.Error("no row of the songs found is a Tab stop")
	}
}

// "Added by" belongs to the song, so it follows the song to where the sort
// puts it.
func TestPageAddedByFollowsTheSongsListed(t *testing.T) {
	songs := viewSongs()
	for i := range songs {
		songs[i].CatalogID = "c" + songs[i].ID
	}
	d := catalogDetail("pl.u-9", "Shared mix", songs...)
	d.Kind = "user-shared"
	r := newPageRig(t, "pl.u-9", d)
	r.api.collab["pl.u-9"] = services.PlaylistCollaboration{
		Collaborative: true,
		OwnerID:       "sp.1",
		Collaborators: []services.SocialProfile{
			{ID: "sp.1", Name: "Ada", Handle: "ada"},
			{ID: "sp.2", Name: "Bo", Handle: "bo"},
		},
		Viewer: services.RoleListener,
		AddedBy: []services.TrackAddedBy{
			{SongID: "ca", ProfileID: "sp.1"}, // Charlie
			{SongID: "cb", ProfileID: "sp.2"}, // alpha
		},
	}
	r.shows("Added by", "Charlie", "Bo")
	if got := r.order("Ada", "Bo"); !slices.Equal(got, []string{"Ada", "Bo"}) {
		t.Fatalf("who added them, in the playlist's order: %v", got)
	}
	r.click("Title") // alpha (Bo) first, Charlie (Ada) third
	r.tt.Settle()
	if got := r.order("Ada", "Bo"); !slices.Equal(got, []string{"Bo", "Ada"}) {
		t.Errorf("who added them, in the listed order: %v", got)
	}
}

func TestPageEditModeIsUnsortedUnfilteredAndHidesTheControls(t *testing.T) {
	r := viewRig(t)
	r.find("e")
	r.click("Title")
	r.click("Title")
	r.tt.Settle()
	r.click("Edit")
	r.shows("Done", "Edit the songs in Road Trip")
	for _, gone := range []string{"Find in playlist", "Songs in Road Trip"} {
		if r.tt.HasText(gone) {
			t.Errorf("edit mode still shows %q", gone)
		}
	}
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleSearch, "Find songs in this playlist"); ok {
		t.Error("the find landmark is in edit mode")
	}
	if _, ok := tr.Find(ui.RoleButton, "Title"); ok {
		t.Error("a sort button is in edit mode")
	}
	if got := r.order(viewNames...); !slices.Equal(got, viewNames) {
		t.Errorf("the editor's order: %v, want the playlist's", got)
	}

	// Back: the controls are back, with nothing sorted or found.
	r.click("Done")
	r.shows("Find in playlist", "Songs in Road Trip")
	if got := r.order(viewNames...); !slices.Equal(got, viewNames) {
		t.Errorf("after Done: %v", got)
	}
	if r.tt.HasText("of 4 songs") {
		t.Error("the query came back")
	}
	if got := sortOfHeader(t, access.Read(t, r.tt.Tester), "Title"); got != "" {
		t.Errorf("Title is still %q", got)
	}
}

func TestPageCatalogPlaylistSortsAndFiltersToo(t *testing.T) {
	r := newPageRig(t, "pl.1", catalogDetail("pl.1", "Apple's Mix", viewSongs()...))
	r.shows("Charlie", "Find in playlist")
	r.click("Title")
	r.tt.Settle()
	if got := r.order(viewNames...); !slices.Equal(got, []string{"alpha", "Bravo", "Charlie", "Échelle"}) {
		t.Errorf("a catalogue playlist by title: %v", got)
	}
	r.find("mia")
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	r.click("Play Apple's Mix")
	r.tt.Until("the play", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got != "[b d] start=0 from=playlist:pl.1" {
		t.Errorf("Play: %q", got)
	}
	// Nothing in it can be removed.
	if err := r.tt.RightClick("alpha"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the row menu", r.tt.Shows("Add to playlist"))
	if r.tt.HasText("Remove from playlist") {
		t.Error("a catalogue playlist offers Remove")
	}
}

func TestPageWithNoSongsHasNoFinder(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip"))
	r.shows("This playlist is empty")
	if r.tt.HasText("Find in playlist") {
		t.Error("a field to search no songs")
	}
}

// bigSongs are n songs whose titles and lengths both rise with their number.
func bigSongs(n int) []services.Song {
	out := make([]services.Song, n)
	for i := range out {
		s := pgSong(fmt.Sprintf("s%03d", i+1), fmt.Sprintf("Track %03d", i+1), fmt.Sprintf("Artist %d", i%7), "Album")
		s.DurationMs = int64(i+1) * 1000
		out[i] = s
	}
	return out
}

// Above 200 rows the list is virtual: only the rows in view are built, and
// the sort and the filter work on all of them.
func TestPageSortsAndFiltersAVirtualList(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Big Mix", bigSongs(300)...))
	r.shows("Track 001", "Find in playlist")
	if r.tt.HasText("Track 300") {
		t.Fatal("the list is not virtual: the last row is built")
	}
	tr := access.Read(t, r.tt.Tester)
	if tbl, ok := tr.Find(ui.RoleTable, "Songs in Big Mix"); !ok || tbl.RowCount != 301 {
		t.Fatalf("the table: %+v, want 301 rows with the header", tbl)
	}

	// The longest first: the last song is on top.
	r.click("Duration")
	r.click("Duration")
	r.tt.Settle()
	r.shows("Track 300")
	if r.tt.HasText("Track 001") {
		t.Error("the first song is still built at the top of a descending list")
	}

	// A query narrows 300 to thirteen, which are no longer virtual: its
	// words are "track" and "29", so 029, 129, 229 and 290 to 299.
	r.find("track 29")
	r.tt.Until("the count", r.tt.Shows("13 of 300 songs"))
	tr = access.Read(t, r.tt.Tester)
	if tbl, ok := tr.Find(ui.RoleTable, "Songs in Big Mix"); !ok || tbl.RowCount != 14 {
		t.Errorf("the table of the songs found: %+v, want 14 rows with the header", tbl)
	}
	r.shows("Track 299", "Track 290", "Track 029")
	if r.tt.HasText("Track 300") || r.tt.HasText("Track 030") {
		t.Error("a song that does not match is listed")
	}
	// Still longest first.
	if got := r.order("Track 299", "Track 290", "Track 229", "Track 129", "Track 029"); !slices.Equal(got, []string{"Track 299", "Track 290", "Track 229", "Track 129", "Track 029"}) {
		t.Errorf("order of the songs found: %v", got)
	}
}
